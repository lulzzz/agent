package cli

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cheggaaa/pb"
	"github.com/nightlyone/lockfile"
	"github.com/subutai-io/agent/config"
	"github.com/subutai-io/agent/db"
	"github.com/subutai-io/agent/lib/container"
	"github.com/subutai-io/agent/lib/gpg"
	"github.com/subutai-io/agent/lib/template"
	"github.com/subutai-io/agent/log"
)

var (
	lock lockfile.Lockfile
)

type templ struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Owner      []string          `json:"owner"`
	Version    string            `json:"version"`
	File       string            `json:"filename"`
	Signatures map[string]string `json:"signature"`
	Branch     string
	Local      bool
	Hash       struct {
		Md5    string
		Sha256 string
	} `json:"hash"`
}

func (t *templ) getInfo(kurjun *http.Client, token string) error {
	var list []templ
	url := config.CDN.Kurjun + "/template/info?name=" + t.Name + "&version=" + t.Version + "&token=" + token
	if len(t.ID) != 0 {
		url = config.CDN.Kurjun + "/template/info?id=" + t.ID + "&token=" + token
	}
	response, err := kurjun.Get(url)
	log.Debug(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if err == nil && len(t.ID) == 0 && response.StatusCode == 404 {
		log.Warn("Requested template version not found, getting available")
		log.Debug(config.CDN.Kurjun + "/template/info?name=" + t.Name + "&token=" + token)
		response, err = kurjun.Get(config.CDN.Kurjun + "/template/info?name=" + t.Name + "&token=" + token)
		if err != nil {
			return err
		}
	}
	if response.StatusCode != 200 {
		return err
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil || log.Check(log.WarnLevel, "Parsing response body", json.Unmarshal(body, &list)) {
		return err
	}
	if len(list) > 1 {
		fmt.Printf("There are multiple templates named %s in repository\nPlease run `subutai import id:<id>` with id from list:\n" + t.Name)
		for _, v := range list {
			fmt.Printf("%s (owner: %s)\n", v.ID, v.Owner)
		}
		os.Exit(0)
	}
	*t = list[0]
	log.Debug("Name: " + t.Name + ", version: " + t.Version)
	return nil
}

func (t *templ) verifySignature() error {
	if len(t.Signatures) == 0 {
		return nil
	}
	for owner, signature := range t.Signatures {
		for _, key := range gpg.KurjunUserPK(owner) {
			if t.ID == gpg.VerifySignature(key, signature) {
				log.Info("Template's owner signature verified")
				log.Debug("Signature belongs to " + owner)
				return nil
			}
			log.Debug("Signature does not match with template id")
		}
	}
	return fmt.Errorf("Failed to verify signature")
}

func (t *templ) verifyHash() error {
	hash := md5sum(config.Agent.LxcPrefix + "tmpdir/" + t.ID)
	if t.ID == hash || t.Hash.Md5 == hash {
		return nil
	}
	return fmt.Errorf("Failed to verify hash sum")
}

func (t *templ) exists() bool {
	var response string
	files, _ := ioutil.ReadDir(config.Agent.LxcPrefix + "tmpdir")
	for _, f := range files {
		if strings.HasPrefix(f.Name(), t.Name) {
			if len(t.ID) == 0 {
				fmt.Print("Cannot verify local template. Trust anyway? (y/n)")
				_, err := fmt.Scanln(&response)
				log.Check(log.FatalLevel, "Reading input", err)
				if response == "y" {
					t.File = f.Name()
					return true
				}
				return false
			}
			hash := md5sum(config.Agent.LxcPrefix + "tmpdir/" + f.Name())
			if t.ID == hash || t.Hash.Md5 == hash {
				return true
			}
		}
	}
	return false
}

func md5sum(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// lockSubutai creates lock file for period of import for certain template to prevent conflicts during write operation
func lockSubutai(file string) bool {
	lock, err := lockfile.New("/var/run/lock/subutai." + file)
	if log.Check(log.DebugLevel, "Init lock "+file, err) {
		return false
	}

	err = lock.TryLock()
	if log.Check(log.DebugLevel, "Locking file "+file, err) {
		if p, err := lock.GetOwner(); err == nil {
			cmd, err := ioutil.ReadFile(fmt.Sprintf("/proc/%v/cmdline", p.Pid))
			if err != nil || !(strings.Contains(string(cmd), "subutai") && strings.Contains(string(cmd), "import")) {
				log.Check(log.DebugLevel, "Removing broken lockfile /var/run/lock/subutai."+file, os.Remove("/var/run/lock/subutai."+file))
			}
		}
		return false
	}
	return true
}

// unlockSubutai removes lock file
func unlockSubutai() {
	lock.Unlock()
}

func (t *templ) download(kurjun *http.Client, token string, torrent bool) error {
	if len(t.ID) == 0 {
		return fmt.Errorf("Download failed: empty template id")
	}
	out, err := os.Create(config.Agent.LxcPrefix + "tmpdir/" + t.ID)
	if err != nil {
		return err
	}
	defer out.Close()

	url := config.CDN.Kurjun + "/template/download?id=" + t.ID + "&token=" + token
	if len(t.Owner) > 0 {
		url = config.CDN.Kurjun + "/template/download?id=" + t.ID + "&owner=" + t.Owner[0] + "&token=" + token
	}
	response, err := kurjun.Get(url)
	if err != nil {
		return err
	}

	defer response.Body.Close()
	bar := pb.New(int(response.ContentLength)).SetUnits(pb.U_BYTES)
	if response.ContentLength <= 0 {
		bar.NotPrint = true
	}
	bar.Start()
	rd := bar.NewProxyReader(response.Body)
	_, err = io.Copy(out, rd)
	if err != nil {
		return err
	}
	bar.Finish()
	return nil
}

// LxcImport function deploys a Subutai template on a Resource Host. The import algorithm works with both the global template repository and a local directory
// to provide more flexibility to enable working with published and custom local templates. Official published templates in the global repository have a overriding scope
// over custom local artifacts if there's any template naming conflict.
//
// If Internet access is lost, or it is not possible to upload custom templates to the repository, the filesystem path `/mnt/lib/lxc/tmpdir/` could be used as local repository;
// the import sub command checks this directory if a requested published template or the global repository is not available.
//
// The import binding handles security checks to confirm the authenticity and integrity of templates. Besides using strict SSL connections for downloads,
// it verifies the fingerprint and its checksum for each template: an MD5 hash sum signed with author's GPG key. Import executes different integrity and authenticity checks of the template
// transparent to the user to protect system integrity from all possible risks related to template data transfers over the network.
//
// The template's version may be specified with the `-v` option. By default import retrieves the latest available template version from repository.
// The repository supports public, group private (shared), and private files. Import without specifying a security token can only access public templates.
//
// `subutai import management` is a special operation which differs from the import of other templates. Besides the usual template deployment operations,
// "import management" demotes the template, starts its container, transforms the host network, and forwards a few host ports, etc.
func LxcImport(name, version, token string) {
	// var kurjun *http.Client
	var t templ
	kurjun, err := config.CheckKurjun()
	if err != nil || kurjun == nil {
		log.Warn(err)
		t.Local = true
	}

	if container.IsContainer(name) && name == "management" && len(token) > 1 {
		gpg.ExchageAndEncrypt("management", token)
		return
	}

	if container.IsContainer(t.Name) {
		log.Info(t.Name + " instance exist")
		return
	}

	if id := strings.Split(name, "id:"); len(id) > 1 {
		t.ID = id[1]
	} else if line := strings.Split(t.Name, "/"); len(line) > 1 {
		t.Owner = append(t.Owner, line[0])
		t.Name = line[1]
	} else {
		t.Name = name
	}

	log.Info("Importing " + name)
	for !lockSubutai(t.Name + ".import") {
		time.Sleep(time.Second)
	}
	defer unlockSubutai()

	t.Version = config.Template.Version
	t.Branch = config.Template.Branch
	if len(version) != 0 {
		t.Version = version
	}

	log.Info("Version: " + t.Version + ", branch: " + t.Branch)

	if !t.Local {
		if err := t.getInfo(kurjun, token); err != nil {
			log.Warn(err)
			t.Local = true
		}
	}

	if !t.Local && len(t.Signatures) == 0 {
		log.Error("Template is not signed")
	} else if !t.Local {
		if err := t.verifySignature(); err != nil {
			log.Error(err)
		} else {
			if err := t.download(kurjun, token, false); err != nil {
				log.Error(err)
			}
			if err := t.verifyHash(); err != nil {
				log.Error(err)
			}
			log.Info("File integrity verified")
		}
	} else if t.Local && !t.exists() {
		log.Error("Cannot find template")
	}

	log.Info("Unpacking template " + t.Name)
	parent, err := template.Extract(t.ID, t.Name)
	log.Check(log.ErrorLevel, "Extracting tar archive", err)
	if len(parent) != 0 {
		log.Info("Parent template required: " + parent)
		LxcImport(parent, "", token)
	}

	bolt, err := db.New()
	log.Check(log.WarnLevel, "Opening database", err)
	log.Check(log.WarnLevel, "Writing container data to database", bolt.TemplateAdd(t.Name, t.ID))
	parentID := bolt.TemplateID(parent)
	log.Check(log.WarnLevel, "Closing database", bolt.Close())

	log.Info("Installing template " + t.ID)
	template.Deploy(parentID, t.ID)

	log.Info("Setting configuration")
	if t.Name == "management" {
		template.MngInit(t.ID)
	} else {
		template.SetConfig(t.ID, t.Name)
	}
}

/*
fs: uuid
commands: name
*/
