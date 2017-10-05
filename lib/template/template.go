// Packag template works with template deployment, configuration and initialisation
package template

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"

	"code.cloudfoundry.org/archiver/extractor"
	"github.com/subutai-io/agent/config"
	"github.com/subutai-io/agent/db"
	"github.com/subutai-io/agent/lib/container"
	"github.com/subutai-io/agent/lib/fs"
	"github.com/subutai-io/agent/lib/gpg"
	"github.com/subutai-io/agent/lib/net"
	"github.com/subutai-io/agent/log"
)

// Extract is unpacking template archive and returns template parent
func Extract(id, name string) (string, error) {
	tgz := extractor.NewTgz()
	tmpdir := config.Agent.LxcPrefix + "tmpdir/" + id + ".dir"
	log.Debug(config.Agent.LxcPrefix + "tmpdir/" + id + " to " + tmpdir)
	if err := tgz.Extract(config.Agent.LxcPrefix+"tmpdir/"+id, tmpdir); err != nil {
		return "", err
	}
	parent := container.GetConfigItem(tmpdir+"/config", "subutai.parent")
	if parent != "" && parent != name && !container.IsTemplate(parent) {
		return parent, nil
	}
	return "", nil
}

// Deploy installs downloaded and unpacked templates to the system
func Deploy(parent, child string) {
	delta := map[string][]string{
		child + ".dir/deltas/rootfs.delta": {parent + "/rootfs", child},
		child + ".dir/deltas/home.delta":   {parent + "/home", child},
		child + ".dir/deltas/opt.delta":    {parent + "/opt", child},
		child + ".dir/deltas/var.delta":    {parent + "/var", child},
	}

	fs.SubvolumeCreate(config.Agent.LxcPrefix + child)

	p := true
	if parent == child || parent == "" {
		p = false
	}

	for delta, path := range delta {
		fs.Receive(config.Agent.LxcPrefix+path[0], config.Agent.LxcPrefix+path[1], delta, p)
	}

	for _, file := range []string{"config", "fstab", "packages"} {
		fs.Copy(config.Agent.LxcPrefix+"tmpdir/"+child+".dir/"+file, config.Agent.LxcPrefix+child+"/"+file)
	}
	log.Check(log.WarnLevel, "Removing extraction directory", os.RemoveAll(config.Agent.LxcPrefix+"tmpdir/"+child+".dir"))
}

// SetConfig sets default template configuration after import
func SetConfig(id, name string) {
	container.SetContainerConf(id, [][]string{
		{"lxc.include", ""},
	})

	container.SetContainerConf(id, [][]string{
		{"lxc.rootfs", config.Agent.LxcPrefix + id + "/rootfs"},
		{"lxc.rootfs.mount", config.Agent.LxcPrefix + id + "/rootfs"},
		{"lxc.mount", config.Agent.LxcPrefix + id + "/fstab"},
		{"lxc.hook.pre-start", ""},
		{"lxc.include", config.Agent.AppPrefix + "share/lxc/config/ubuntu.common.conf"},
		{"lxc.include", config.Agent.AppPrefix + "share/lxc/config/ubuntu.userns.conf"},
		{"subutai.config.path", config.Agent.AppPrefix + "etc"},
		{"lxc.network.script.up", config.Agent.AppPrefix + "bin/create_ovs_interface"},
		{"lxc.mount.entry", config.Agent.LxcPrefix + id + "/home home none bind,rw 0 0"},
		{"lxc.mount.entry", config.Agent.LxcPrefix + id + "/opt opt none bind,rw 0 0"},
		{"lxc.mount.entry", config.Agent.LxcPrefix + id + "/var var none bind,rw 0 0"},
	})
}

// Mac function generates random mac address for LXC containers
func Mac() string {
	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	log.Check(log.ErrorLevel, "Generating random mac", err)
	return fmt.Sprintf("00:16:3e:%02x:%02x:%02x", buf[3], buf[4], buf[5])
}

// MngInit performs initial operations for SS Management deployment
func MngInit(id string) {
	fs.ReadOnly(id, false)
	container.SetContainerUID("management")
	container.SetContainerConf("management", [][]string{
		{"lxc.network.hwaddr", Mac()},
		{"lxc.network.veth.pair", "management"},
		{"lxc.network.script.up", config.Agent.AppPrefix + "bin/create_ovs_interface"},
		{"lxc.network.link", ""},
		{"lxc.mount", config.Agent.LxcPrefix + id + "/fstab"},
		{"lxc.rootfs", config.Agent.LxcPrefix + id + "/rootfs"},
		{"lxc.rootfs.mount", config.Agent.LxcPrefix + id + "/rootfs"},
		// TODO following lines kept for back compatibility with old templates, should be deleted when all templates will be replaced.
		{"lxc.mount.entry", config.Agent.LxcPrefix + id + "/home home none bind,rw 0 0"},
		{"lxc.mount.entry", config.Agent.LxcPrefix + id + "/opt opt none bind,rw 0 0"},
		{"lxc.mount.entry", config.Agent.LxcPrefix + id + "/var var none bind,rw 0 0"},
	})
	gpg.GenerateKey("management")
	container.SetApt("management")
	container.SetDNS("management")
	container.AddMetadata("management", map[string]string{"ip": "10.10.10.1"})
	container.Start("management")

	//TODO move mapping functions from cli package and get rid of exec
	log.Check(log.WarnLevel, "Exposing port 8443",
		exec.Command("subutai", "map", "tcp", "-i", "10.10.10.1:8443", "-e", "8443").Run())
	log.Check(log.WarnLevel, "Exposing port 8444",
		exec.Command("subutai", "map", "tcp", "-i", "10.10.10.1:8444", "-e", "8444").Run())
	log.Check(log.WarnLevel, "Exposing port 8086",
		exec.Command("subutai", "map", "tcp", "-i", "10.10.10.1:8086", "-e", "8086").Run())

	bolt, err := db.New()
	log.Check(log.WarnLevel, "Opening database", err)
	log.Check(log.WarnLevel, "Writing container data to database", bolt.ContainerAdd("management", map[string]string{"ip": "10.10.10.1"}))
	log.Check(log.WarnLevel, "Closing database", bolt.Close())

	log.Info("********************")
	log.Info("Subutai Management UI will be shortly available at https://" + net.GetIp() + ":8443")
	log.Info("login: admin")
	log.Info("password: secret")
	log.Info("********************")
}

// MngDel removes Management network interfaces, resets dhcp client
func MngDel() {
	exec.Command("ovs-vsctl", "del-port", "wan", "management").Run()
	exec.Command("ovs-vsctl", "del-port", "wan", "mng-gw").Run()
}
