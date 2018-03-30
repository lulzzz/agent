package fs

import (
	"os/exec"
	"strings"
	"github.com/subutai-io/agent/log"
)

func IsDatasetReadOnly(dataset string) bool {
	out, err := exec.Command("/bin/bash", "-c", "zfs get readonly -H "+dataset+" | awk '{print $3}' ").CombinedOutput()
	output := strings.TrimSpace(string(out))
	log.Check(log.FatalLevel, "Getting zfs dataset readonly property "+dataset+"\n"+output, err)
	return output == "on"
}

func DatasetExists(dataset string) bool {
	out, err := exec.Command("/bin/bash", "-c", "zfs list -H "+dataset).CombinedOutput()
	log.Check(log.DebugLevel, "Checking zfs dataset existence "+dataset+"\n"+string(out), err)
	return err == nil
}

func SetDatasetReadOnly(dataset string) {
	out, err := exec.Command("/bin/bash", "-c", "zfs set readonly=on "+dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Setting zfs dataset readonly "+dataset+"\n"+string(out), err)
}

func RemoveDataset(dataset string) {
	out, err := exec.Command("/bin/bash", "-c", "zfs destroy -r "+dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Removing zfs dataset "+dataset+"\n"+string(out), err)
}

func CreateDataset(dataset string) {
	out, err := exec.Command("/bin/bash", "-c", "zfs create "+dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Creating zfs dataset "+dataset+"\n"+string(out), err)
}

func ReceiveStream(delta string, destDataset string) {
	out, err := exec.Command("/bin/bash", "-c", "zfs receive "+destDataset+" < "+delta).CombinedOutput()
	log.Check(log.ErrorLevel, "Creating zfs stream "+destDataset+" < "+delta+"\n"+string(out), err)
}

func SetMountpoint(dataset string, mountpoint string) {
	out, err := exec.Command("/bin/bash", "-c", "zfs set mountpoint="+mountpoint+" "+dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Setting mountpoint to dataset "+mountpoint+" -> "+dataset+"\n"+string(out), err)
}
