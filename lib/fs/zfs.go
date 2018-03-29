package fs

import (
	"os/exec"
	"strings"
	"github.com/subutai-io/agent/log"
)

func IsDatasetReadOnly(dataset string) bool {
	out, err := exec.Command("/bin/bash", "-c", "zfs get readonly -H "+dataset+" | awk '{print $3}' ").CombinedOutput()
	output := strings.TrimSpace(string(out))
	log.Check(log.FatalLevel, "Getting zfs readonly property "+output, err)
	return output == "on"
}

func DatasetExists(dataset string) bool {
	_, err := exec.Command("/bin/bash", "-c", "zfs list -H "+dataset).CombinedOutput()
	return err == nil
}

func SetDatasetReadOnly(dataset string) {
	out, err := exec.Command("/bin/bash", "-c", "zfs set readonly=on "+dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Setting zfs readonly property "+string(out), err)
}

func RemoveDataset(dataset string) {
	out, err := exec.Command("/bin/bash", "-c", "zfs destroy -r "+dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Removing zfs dataset "+string(out), err)
}

func CreateDataset(dataset string) {
	out, err := exec.Command("/bin/bash", "-c", "zfs create "+dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Creating zfs dataset "+string(out), err)
}

func ReceiveStream(delta string, destDataset string) {
	out, err := exec.Command("/bin/bash", "-c", "cat "+delta+" | zfs receive "+destDataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Creating zfs stream "+string(out), err)
}
