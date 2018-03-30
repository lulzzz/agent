package fs

import (
	"os/exec"
	"strings"
	"github.com/subutai-io/agent/log"
)

func IsDatasetReadOnly(dataset string) bool {
	out, err := exec.Command("zfs", "get", "readonly", "-H", dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Reading zfs dataset readonly property "+string(out), err)
	output := string(out)
	fields := strings.Fields(output)
	readonly := false
	if len(fields) > 2 {
		readonly = fields[2] == "on"
	} else {
		log.Error("Failed to parse output " + output)
	}

	return readonly
}

func DatasetExists(dataset string) bool {
	out, err := exec.Command("zfs", "list", "-H", dataset).CombinedOutput()
	log.Check(log.DebugLevel, "Checking zfs dataset existence "+dataset+"\n"+string(out), err)
	return err == nil
}

func SetDatasetReadOnly(dataset string) {
	out, err := exec.Command("zfs", "set", "readonly=on", dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Setting zfs dataset readonly "+dataset+"\n"+string(out), err)
}
func SetDatasetReadWrite(dataset string) {
	out, err := exec.Command("zfs", "set", "readonly=off", dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Setting zfs dataset read-write "+dataset+"\n"+string(out), err)
}

func RemoveDataset(dataset string) {
	out, err := exec.Command("zfs", "destroy", "-r", dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Removing zfs dataset "+dataset+"\n"+string(out), err)
}

func CreateDataset(dataset string) {
	out, err := exec.Command("zfs", "create", dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Creating zfs dataset "+dataset+"\n"+string(out), err)
}

//have to use bash shell to send piped command
func ReceiveStream(delta string, destDataset string) {
	out, err := exec.Command("/bin/bash", "-c", "zfs receive "+destDataset+" < "+delta).CombinedOutput()
	log.Check(log.ErrorLevel, "Creating zfs stream "+destDataset+" < "+delta+"\n"+string(out), err)
}

func SetMountpoint(dataset string, mountpoint string) {
	out, err := exec.Command("zfs", "set", "mountpoint="+mountpoint, dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Setting mountpoint to dataset "+mountpoint+" -> "+dataset+"\n"+string(out), err)
}
