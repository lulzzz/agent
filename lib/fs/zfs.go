package fs

import (
	"os/exec"
	"strings"
	"github.com/subutai-io/agent/log"
)

func IsDatasetReadOnly(dataset string) bool {
	out, err := exec.Command("/bin/bash", "-c", "/sbin/zfs get readonly -H "+dataset+" | awk '{print $3}' ").CombinedOutput()
	output := strings.TrimSpace(string(out))
	log.Check(log.FatalLevel, "Getting zfs readonly property "+output, err)
	return output == "on"
}

func RemoveDataset(dataset string) {
	out, err := exec.Command("/bin/bash", "-c", "/sbin/zfs destroy -r "+dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Removing zfs dataset "+string(out), err)
}

func CreateDataset(dataset string) {
	out, err := exec.Command("/bin/bash", "-c", "/sbin/zfs create "+dataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Creating zfs dataset "+string(out), err)
}

func ReceiveStream(delta string, destDataset string) {
	out, err := exec.Command("/bin/bash", "-c", "cat "+delta+" | /sbin/zfs receive "+destDataset).CombinedOutput()
	log.Check(log.ErrorLevel, "Creating zfs stream "+string(out), err)
}
