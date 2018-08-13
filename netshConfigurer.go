package main

import(
	"bufio"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"syscall"

	arp "github.com/mdlayher/arp"
)

const (
	netsh_arpReplyOp = 2
)

type NetshConfigurer struct {
	*IPConfiguration
	arpClient    *arp.Client
}

func NewNetshConfigurer(config *IPConfiguration) (*NetshConfigurer, error){
	c := &NetshConfigurer{IPConfiguration : config}

	arpClient, err := arp.Dial(&c.iface)
	if err != nil {
		log.Printf("Problems with producing the arp client: %s", err)
		return nil, err
	}
	c.arpClient = arpClient

	return c, nil
}


func (c *NetshConfigurer) ARPSendGratuitous() error {
	gratuitousPackage, err := arp.NewPacket(
		arpReplyOp,
		c.iface.HardwareAddr,
		c.vip,
		ethernetBroadcast,
		net.IPv4bcast,
	)

	if err != nil {
		log.Printf("Gratuitous arp package is malformed: %s", err)
		return err
	}

	err = c.arpClient.WriteTo(gratuitousPackage, ethernetBroadcast)
	if err != nil {
		log.Printf("Cannot send gratuitous arp message: %s", err)
		return err
	}

	return nil
}

func (c *NetshConfigurer) QueryAddress() bool {
	cmd := exec.Command("netsh", "interface", "ipv4", "show", "config", c.iface.Name)

	lookup := c.GetCIDR()
	result := false

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}

	err = cmd.Start()
	if err != nil {
		panic(err)
	}

	scn := bufio.NewScanner(stdout)

	for scn.Scan() {
		line := scn.Text()
		if strings.Contains(line, lookup) {
			result = true
		}
	}

	cmd.Wait()

	return result
}

func (c *NetshConfigurer) ConfigureAddress() bool {
	log.Printf("Configuring address %s on %s", c.GetCIDR(), c.iface.Name)

	result:= c.runAddressConfiguration("add")

	if(result == true){
		// For now it is save to say that also working even if a
		// gratuitous arp message could not be send but logging an
		// errror should be enough.
		c.ARPSendGratuitous()
	}

	return result
}

func (c *NetshConfigurer) DeconfigureAddress() bool {
	log.Printf("Removing address %s on %s", c.GetCIDR(), c.iface.Name)
	return c.runAddressConfiguration("delete")
}

func (c *NetshConfigurer) runAddressConfiguration(action string) bool {
	// netsh interface ipv4 add address "INTERFACE NAME" address=192.168.0.12 mask=255.255.255.0 gateway=192.168.0.1
	// netsh interface ipv4 delete address "INTERFACE NAME" address=192.168.0.12 
	// gateway in delete is optional
	cmd := exec.Command("ip", "addr", action,
		c.GetCIDR(),
		"dev", c.iface.Name)
	err := cmd.Run()

	switch exit := err.(type) {
	case *exec.ExitError:
		if status, ok := exit.Sys().(syscall.WaitStatus); ok {
			if status.ExitStatus() == 2 {
				// Already exists
				return true
			} else {
				log.Printf("Got error %s", status)
			}
		}

		return false
	}
	if err != nil {
		log.Printf("Error running ip address %s %s on %s: %s",
			action, c.vip, c.iface.Name, err)
		return false
	}
	return true
}

func (c *NetshConfigurer) GetCIDR() string {
	return fmt.Sprintf("%s/%d", c.vip.String(), NetmaskSize(c.netmask))
}

func (c *NetshConfigurer) cleanupArp() {
	c.arpClient.Close()
}