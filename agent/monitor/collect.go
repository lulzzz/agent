// Package monitor gathers system statistics information and sends it to time-series database
package monitor

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/influxdata/influxdb/client/v2"

	"github.com/subutai-io/agent/config"
	"github.com/subutai-io/agent/lib/container"
	"github.com/subutai-io/agent/log"
	"github.com/subutai-io/agent/agent/utils"
)

var (
	traff     = []string{"in", "out"}
	cgtype    = []string{"cpuacct", "memory"}
	metrics   = []string{"total", "used", "available"}
	cpu       = []string{"user", "nice", "system", "idle", "iowait"}
	lxcmemory = map[string]bool{"cache": true, "rss": true, "Cached": true, "MemFree": true}
	memory    = map[string]bool{"Active": true, "Buffers": true, "Cached": true, "MemFree": true}
)

// Collect collecting performance statistic from Resource Host and Subutai Containers.
// It sends this information to InfluxDB server using credentials from configuration file.
func Collect() {

	for {

		doCollect()

		time.Sleep(time.Second * 30)
	}
}

func doCollect() {

	influx, err := utils.InfluxDbClient()
	if err == nil {
		defer influx.Close()
	}

	log.Check(log.WarnLevel, "Entering metrics collection routine", err)

	if err == nil {

		_, _, err := influx.Ping(time.Second)

		log.Check(log.WarnLevel, "Pinging InfluxDB server", err)

		if err == nil {

			bp, err := client.NewBatchPoints(client.BatchPointsConfig{Database: config.Influxdb.Db, RetentionPolicy: "hour"})

			log.Check(log.WarnLevel, "Preparing metrics batch", err)

			if err == nil {

				netStat(bp)
				cgroupStat(bp)
				btrfsStat(bp)
				diskFree(bp)
				cpuStat(bp)
				memStat(bp)

				err = influx.Write(bp)

				log.Check(log.WarnLevel, "Writing metrics batch", err)
			}
		}
	}
}

func parsefile(bp client.BatchPoints, hostname, lxc, cgtype, filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(bufio.NewReader(file))
	for scanner.Scan() {
		line := strings.Split(scanner.Text(), " ")
		if value, err := strconv.Atoi(line[1]); err == nil {
			if cgtype == "memory" && lxcmemory[line[0]] {
				point, err := client.NewPoint("lxc_"+cgtype,
					map[string]string{"hostname": lxc, "type": line[0]},
					map[string]interface{}{"value": value},
					time.Now())
				if err == nil {
					bp.AddPoint(point)
				}
			} else if cgtype == "cpuacct" {
				point, err := client.NewPoint("lxc_cpu",
					map[string]string{"hostname": lxc, "type": line[0]},
					map[string]interface{}{"value": value / runtime.NumCPU()},
					time.Now())
				bp.AddPoint(point)
				if err == nil {
					bp.AddPoint(point)
				}
			}
		}
	}

}

func cgroupStat(bp client.BatchPoints) {
	hostname, err := os.Hostname()
	log.Check(log.DebugLevel, "Getting hostname of the system", err)
	for _, item := range cgtype {
		path := "/sys/fs/cgroup/" + item + "/lxc/"
		files, err := ioutil.ReadDir(path)
		if err == nil {
			for _, f := range files {
				if f.IsDir() {
					parsefile(bp, hostname, f.Name(), item, path+f.Name()+"/"+item+".stat")
				}
			}
		}
	}
}

func netStat(bp client.BatchPoints) {
	lxcnic := make(map[string]string)
	files, err := ioutil.ReadDir(config.Agent.LxcPrefix)
	if err == nil {
		for _, f := range files {
			lxcnic[container.GetConfigItem(config.Agent.LxcPrefix+f.Name()+"/config", "lxc.network.veth.pair")] = f.Name()
		}
	}

	out, err := ioutil.ReadFile("/proc/net/dev")
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	traffic := make([]int, 2)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), ":") {
			line := strings.Fields(scanner.Text())
			traffic[0], err = strconv.Atoi(line[1])
			log.Check(log.DebugLevel, "Parsing network stat file from proc", err)
			traffic[1], err = strconv.Atoi(line[9])
			log.Check(log.DebugLevel, "Parsing network stat file from proc", err)

			nicname := strings.Split(line[0], ":")[0]
			metric := "host_net"
			hostname, err := os.Hostname()
			log.Check(log.DebugLevel, "Getting hostname of the system", err)
			if lxcnic[nicname] != "" {
				metric = "lxc_net"
				hostname = lxcnic[nicname]
			}

			for i := range traffic {
				point, err := client.NewPoint(metric,
					map[string]string{"hostname": hostname, "iface": nicname, "type": traff[i]},
					map[string]interface{}{"value": traffic[i] * 8},
					time.Now())
				if err == nil {
					bp.AddPoint(point)
				}
			}
		}
	}
}

func btrfsStat(bp client.BatchPoints) {
	list := make(map[string]string)
	out, err := exec.Command("btrfs", "subvolume", "list", config.Agent.LxcPrefix).Output()
	if log.Check(log.DebugLevel, "Getting BTRFS stats", err) {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.Fields(scanner.Text())
		list["0/"+line[1]] = line[8]
	}
	out, err = exec.Command("btrfs", "qgroup", "show", "-repc", "--raw", config.Agent.LxcPrefix).Output()
	if log.Check(log.DebugLevel, "Getting BTRFS stats", err) {
		return
	}
	scanner = bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.Fields(scanner.Text())
		if path := strings.Split(list[line[0]], "/"); len(path) == 1 {
			if value, err := strconv.Atoi(line[1]); err == nil {
				point, err := client.NewPoint("lxc_disk",
					map[string]string{"hostname": path[0], "mount": "total", "type": "used"},
					map[string]interface{}{"value": value},
					time.Now())
				if err == nil {
					bp.AddPoint(point)
				}
			}
		} else if line[5] == "---" {
			for k, v := range list {
				if v == strings.Split(list[line[0]], "/")[0] && len(k) > 1 {
					exec.Command("btrfs", "qgroup", "create", "1"+k[1:], config.Agent.LxcPrefix).Run()
					exec.Command("btrfs", "qgroup", "assign", line[0], "1"+k[1:], config.Agent.LxcPrefix).Run()
				}
			}
		}
	}
}

func diskFree(bp client.BatchPoints) {
	hostname, err := os.Hostname()
	log.Check(log.DebugLevel, "Getting hostname of the system", err)
	out, err := exec.Command("df", "-B1").Output()
	if log.Check(log.DebugLevel, "Getting disk usage stats", err) {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.Fields(scanner.Text())
		if strings.HasPrefix(line[0], "/dev") {
			for i := range metrics {
				value, err := strconv.Atoi(line[i+1])
				log.Check(log.DebugLevel, "Parsing network disk stats", err)
				point, err := client.NewPoint("host_disk",
					map[string]string{"hostname": hostname, "mount": line[5], "type": metrics[i]},
					map[string]interface{}{"value": value},
					time.Now())
				if err == nil {
					bp.AddPoint(point)
				}
			}
		}
	}
}

func memStat(bp client.BatchPoints) {
	hostname, err := os.Hostname()
	log.Check(log.DebugLevel, "Getting hostname of the system", err)
	if file, err := os.Open("/proc/meminfo"); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(bufio.NewReader(file))
		for scanner.Scan() {
			line := strings.Fields(strings.Replace(scanner.Text(), ":", "", -1))
			if value, err := strconv.Atoi(line[1]); err == nil && memory[line[0]] {
				point, err := client.NewPoint("host_memory",
					map[string]string{"hostname": hostname, "type": line[0]},
					map[string]interface{}{"value": value * 1024},
					time.Now())
				if err == nil {
					bp.AddPoint(point)
				}
			}
		}
	}
}

func cpuStat(bp client.BatchPoints) {
	hostname, err := os.Hostname()
	log.Check(log.DebugLevel, "Getting hostname of the system", err)
	file, err := os.Open("/proc/stat")
	if err != nil {
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(bufio.NewReader(file))
	for scanner.Scan() {
		line := strings.Fields(scanner.Text())
		if line[0] == "cpu" {
			for i := range cpu {
				value, err := strconv.Atoi(line[i+1])
				log.Check(log.DebugLevel, "Parsing network CPU stats from proc", err)
				point, err := client.NewPoint("host_cpu",
					map[string]string{"hostname": hostname, "type": cpu[i]},
					map[string]interface{}{"value": value / runtime.NumCPU()},
					time.Now())
				if err == nil {
					bp.AddPoint(point)
				}
			}
		}
	}
}
