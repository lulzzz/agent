// Packag template works with template deployment, configuration and initialisation
package template

import (
	"crypto/rand"
	"fmt"

	"github.com/subutai-io/agent/config"
	"github.com/subutai-io/agent/lib/fs"
	"github.com/subutai-io/agent/log"
)

// Install deploys downloaded and unpacked templates to the system
func Install(child string) {
	datasets := map[string]string{
		"rootfs.delta": "rootfs",
		"homefs.delta": "home",
		"optfs.delta":  "opt",
		"varfs.delta":  "var",
		//config.Agent.LxcPrefix + "tmpdir/" + child + "/deltas/varfs.delta":  "subutai/fs/" + child + "/var",
	}

	fs.CreateDataset("subutai/fs/" + child)

	for delta, dataset := range datasets {
		fs.ReceiveStream(config.Agent.LxcPrefix+"tmpdir/"+child+"/deltas/"+delta, "subutai/fs/"+child+"/"+dataset)
		fs.SetMountpoint("subutai/fs/"+child+"/"+dataset, config.Agent.LxcPrefix+child+"/"+dataset)
	}

	for _, file := range []string{"config", "fstab", "packages"} {
		fs.Copy(config.Agent.LxcPrefix+"tmpdir/"+child+"/"+file, config.Agent.LxcPrefix+child+"/"+file)
	}
}

// Mac function generates random mac address for LXC containers
func Mac() string {
	buf := make([]byte, 6)
	_, err := rand.Read(buf)
	log.Check(log.ErrorLevel, "Generating random mac", err)
	return fmt.Sprintf("00:16:3e:%02x:%02x:%02x", buf[3], buf[4], buf[5])
}
