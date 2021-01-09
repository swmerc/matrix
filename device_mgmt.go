package main

import (
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

// DeviceMgmtConfig supports posting some stuff to device/+ as well as listening for
// commands on device/cmd.
type DeviceMgmtConfig struct {
	Topic    string `yaml:"topic"`
	Commands []struct {
		Name    string `yaml:"name"`
		CmdLine string `yaml:"cmdline"`
	}
}

func runDeviceMgmt(bmux BrokerMux, cfg DeviceMgmtConfig) {

	if len(cfg.Topic) == 0 {
		return
	}

	//
	// Post our LAN IP
	//
	if ifaces, err := net.Interfaces(); err == nil {
		for _, i := range ifaces {
			if addrs, err := i.Addrs(); err == nil {
				for _, addr := range addrs {
					var ip net.IP
					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					default:
						continue
					}
					if ip.IsLoopback() == false {
						t := bmux.Publish(cfg.Topic+"IP", 1, true, ip.String())
						t.Wait()
						break
					}
				}
			}
		}
	}

	//
	// Process incoming commands
	//
	bmux.Subscribe(cfg.Topic+"cmd", 0, func(client mqtt.Client, msg mqtt.Message) {
		payload := string(msg.Payload())
		log.Infof("command: name=%s", payload)

		// Walk all of the commands and see if the one passed in matches one in the table
		for _, command := range cfg.Commands {
			if strings.Compare(command.Name, payload) == 0 {
				go func() {
					execCommand := exec.Command("/bin/bash", "-c", command.CmdLine)
					raw, err := execCommand.CombinedOutput()
					if err != nil {
						log.Errorf("command: error=%v", err)
					}
					log.Infof("command: result=%s", string(raw))
				}()
				break
			}
		}
	})

	//
	// Sit and spin, posting local time and uptime every hour
	//
	ticker := time.NewTicker(time.Hour)
	uptime := 0

	reportUptime(bmux, cfg.Topic, uptime)

	for {
		select {
		case <-ticker.C:
			reportUptime(bmux, cfg.Topic, uptime)
			uptime = uptime + 1
		}
	}

}

func reportUptime(bmux BrokerMux, baseTopic string, uptime int) {

	t := bmux.Publish(baseTopic+"uptime", 1, true, strconv.Itoa(uptime))
	t.WaitTimeout(10 * time.Second)
	if t.Error() != nil {
		log.Errorf("error: %v", t.Error())
	}

	if timeText, err := time.Now().MarshalText(); err == nil {
		t = bmux.Publish(baseTopic+"time", 1, true, timeText)
		t.WaitTimeout(10 * time.Second)
		if t.Error() != nil {
			log.Errorf("error: %v", t.Error())
		}
	}
}
