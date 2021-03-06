package main

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// getHexPorts gets all open ports as hex strings from /proc/net/tcp
func getHexPorts() (ports []string) {
	data := fileToString("/proc/net/tcp")
	localAddresses := getColumnNoHeader(1, stringToSlice(data))
	portRe := regexp.MustCompile(":([0-9A-F]{4})")
	for _, address := range localAddresses {
		port := portRe.FindString(address)
		if port != "" {
			portString := string(port[1:])
			ports = append(ports, portString)
		}
	}
	return ports
}

// strHexToDecimal converts from string containing hex number to int
func strHexToDecimal(hex string) int {
	portInt, err := strconv.ParseInt(hex, 16, 64)
	if err != nil {
		log.Fatal("Couldn't parse hex number " + hex + ":\n\t" + err.Error())
	}
	return int(portInt)
}

// getOpenPorts gets a list of open/listening ports as integers
func getOpenPorts() (ports []int) {
	for _, port := range getHexPorts() {
		ports = append(ports, strHexToDecimal(port))
	}
	return ports
}

// Port parses /proc/net/tcp to determine if a given port is in an open state
// and returns an error if it is not.
func Port(parameters []string) (exitCode int, exitMessage string) {
	port := parseMyInt(parameters[0])
	open := getOpenPorts()
	for _, p := range open {
		if p == port {
			return 0, ""
		}
	}
	// Convert ports to string to send to genericError
	var strPorts []string
	for _, port := range open {
		strPorts = append(strPorts, fmt.Sprint(port))
	}
	return genericError("Port not open", fmt.Sprint(port), strPorts)
}

// getInterfaces returns a list of network interfaces and handles any associated
// error. Just for DRY.
func getInterfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Fatal("Could not read network interfaces:\n\t" + err.Error())
	}
	return ifaces
}

// Interface detects if a network interface exists,
func Interface(parameters []string) (exitCode int, exitMessage string) {
	// getInterfaceNames returns the names of all network interfaces
	getInterfaceNames := func() (interfaces []string) {
		for _, iface := range getInterfaces() {
			interfaces = append(interfaces, iface.Name)
		}
		return
	}
	name := parameters[0]
	interfaces := getInterfaceNames()
	for _, iface := range interfaces {
		if iface == name {
			return 0, ""
		}
	}
	return genericError("Interface does not exist", name, interfaces)
}

// Up determines if a network interface is up and running or not
func Up(parameters []string) (exitCode int, exitMessage string) {
	// getUpInterfaces returns all the names of the interfaces that are up
	getUpInterfaces := func() (interfaceNames []string) {
		for _, iface := range getInterfaces() {
			if iface.Flags&net.FlagUp != 0 {
				interfaceNames = append(interfaceNames, iface.Name)
			}
		}
		return interfaceNames

	}
	name := parameters[0]
	upInterfaces := getUpInterfaces()
	if strIn(name, upInterfaces) {
		return 0, ""
	}
	return genericError("Interface is not up", name, upInterfaces)
}

// getIPs gets all the associated IP addresses of a given interface as a slice
// of strings, with a given IP protocol version (4|6)
func getInterfaceIPs(name string, version int) (ifaceAddresses []string) {
	// ensure valid IP version
	if version != 4 && version != 6 {
		msg := "Misconfigured JSON: Unsupported IP version: "
		log.Fatal(msg + fmt.Sprint(version))
	}
	for _, iface := range getInterfaces() {
		if iface.Name == name {
			addresses, err := iface.Addrs()
			if err != nil {
				msg := "Could not get network addressed from interface: "
				msg += "\n\tInterface name: " + iface.Name
				msg += "\n\tError: " + err.Error()
				log.Fatal(msg)
			}
			for _, addr := range addresses {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				switch version {
				case 4:
					ifaceAddresses = append(ifaceAddresses, ip.To4().String())
				case 6:
					ifaceAddresses = append(ifaceAddresses, ip.To16().String())
				}
			}
			return ifaceAddresses

		}
	} // will only reach this line if the interface didn't exist
	return ifaceAddresses // will be empty
}

// getIP(exitCode int, exitMessage string) is an abstraction of Ip4 and Ip6
func getIPWorker(name string, address string, version int) (exitCode int, exitMessage string) {
	ips := getInterfaceIPs(name, version)
	if strIn(address, ips) {
		return 0, ""
	}
	return genericError("Interface does not have IP", address, ips)
}

// Ip4 checks to see if this network interface has this ipv4 address
func Ip4(parameters []string) (exitCode int, exitMessage string) {
	return getIPWorker(parameters[0], parameters[1], 4)
}

// Ip6 checks to see if this network interface has this ipv6 address
func Ip6(parameters []string) (exitCode int, exitMessage string) {
	return getIPWorker(parameters[0], parameters[1], 6)
}

// Gateway checks to see that the default gateway has a certain IP
func Gateway(parameters []string) (exitCode int, exitMessage string) {
	// getGatewayAddress filters all gateway IPs for a non-zero value
	getGatewayAddress := func() (addr string) {
		ips := routingTableColumn(1)
		for _, ip := range ips {
			if ip != "0.0.0.0" {
				return ip
			}
		}
		return "0.0.0.0"
	}
	address := parameters[0]
	gatewayIP := getGatewayAddress()
	if address == gatewayIP {
		return 0, ""
	}
	msg := "Gateway does not have address"
	return genericError(msg, address, []string{gatewayIP})
}

// GatewayInterface checks that the default gateway is using a specified interface
func GatewayInterface(parameters []string) (exitCode int, exitMessage string) {
	// getGatewayInterface returns the interface that the default gateway is
	// operating on
	getGatewayInterface := func() (iface string) {
		ips := routingTableColumn(1)
		names := routingTableColumn(1)
		for i, ip := range ips {
			if ip != "0.0.0.0" {
				if len(names) < i {
					msg := "Fewer names in kernel routing table than IPs:"
					msg += "\n\tNames: " + fmt.Sprint(names)
					msg += "\n\tIPs: " + fmt.Sprint(ips)
					log.Fatal()
				}
				return names[i] // interface name
			}
		}
		return ""
	}
	name := parameters[0]
	iface := getGatewayInterface()
	if name == iface {
		return 0, ""
	}
	msg := "Default gateway does not operate on interface"
	return genericError(msg, name, []string{iface})
}

// Host checks if a given host can be resolved.
func Host(parameters []string) (exitCode int, exitMessage string) {
	// resolvable  determines whether a given host can be reached
	resolvable := func(name string) bool {
		_, err := net.LookupHost(name)
		if err == nil {
			return true
		}
		return false
	}
	host := parameters[0]
	if resolvable(host) {
		return 0, ""
	}
	return 1, "Host cannot be resolved: " + host
}

// canConnect tests whether a connection can be made to a given host on its
// given port using protocol ("TCP"|"UDP")
func canConnect(host string, protocol string, timeout time.Duration) bool {
	parseerr := func(err error) {
		if err != nil {
			log.Fatal("Could not parse " + protocol + " address: " + host)
		}
	}
	var conn net.Conn
	var err error
	var timeoutNetwork string = "tcp"
	var timeoutAddress string
	nanoseconds := timeout.Nanoseconds()
	switch protocol {
	case "TCP":
		tcpaddr, err := net.ResolveTCPAddr("tcp", host)
		parseerr(err)
		timeoutAddress = tcpaddr.String()
		if nanoseconds <= 0 {
			conn, err = net.DialTCP(timeoutNetwork, nil, tcpaddr)
		}
	case "UDP":
		timeoutNetwork = "udp"
		udpaddr, err := net.ResolveUDPAddr("udp", host)
		parseerr(err)
		timeoutAddress = udpaddr.String()
		if nanoseconds <= 0 {
			conn, err = net.DialUDP("udp", nil, udpaddr)
		}
	default:
		log.Fatal("Unsupported protocol: " + protocol)
	}
	// if a duration was specified, use it
	if nanoseconds > 0 {
		conn, err = net.DialTimeout(timeoutNetwork, timeoutAddress, timeout)
		if err != nil {
			fmt.Println(err)
		}
	}
	if conn != nil {
		defer conn.Close()
	}
	if err == nil {
		return true
	}
	return false
}

// getConnection(exitCode int, exitMessage string) is an abstraction of TCP and UDP
func getConnectionWorker(host string, protocol string, timeoutstr string) (exitCode int, exitMessage string) {
	dur, err := time.ParseDuration(timeoutstr)
	if err != nil {
		msg := "Configuration error: Could not parse duration: "
		log.Fatal(msg + timeoutstr)
	}
	if canConnect(host, protocol, dur) {
		return 0, ""
	}
	return 1, "Could not connect over " + protocol + " to host: " + host
}

// TCP sees if a given IP/port can be reached with a TCP connection
func TCP(parameters []string) (exitCode int, exitMessage string) {
	return getConnectionWorker(parameters[0], "TCP", "0ns")
}

// UDP is like TCP but with UDP instead.
func UDP(parameters []string) (exitCode int, exitMessage string) {
	return getConnectionWorker(parameters[0], "UDP", "0ns")
}

// tcpTimeout is like TCP, but with a timeout parameter
func tcpTimeout(parameters []string) (exitCode int, exitMessage string) {
	return getConnectionWorker(parameters[0], "TCP", parameters[1])
}

// udpTimeout is like tcpTimeout but with UDP instead.
func udpTimeout(parameters []string) (exitCode int, exitMessage string) {
	return getConnectionWorker(parameters[0], "UDP", parameters[1])
}

// returns a column of the routing table as a slice of strings
func routingTableColumn(column int) []string {
	cmd := exec.Command("route", "-n")
	return commandColumnNoHeader(column, cmd)[1:]
}

// routingTableMatch(exitCode int, exitMessage string) constructs a Worker that returns whether or not the
// given string was found in the given column of the routing table. It is an
// astraction of routingTableDestination, routingTableInterface, and
// routingTableGateway
func routingTableMatch(col int, str string) (exitCode int, exitMessage string) {
	column := routingTableColumn(col)
	if strIn(str, column) {
		return 0, ""
	}
	return genericError("Not found in routing table", str, column)
}

// RoutingTableDestination checks if an IP address is a destination in the
// kernel's IP routing table, as accessed by `route -n`.
func RoutingTableDestination(parameters []string) (exitCode int, exitMessage string) {
	return routingTableMatch(0, parameters[0])
}

// RoutingTableInterface checks if a given name is an interface in the
// kernel's IP routing table, as accessed by `route -n`.
func RoutingTableInterface(parameters []string) (exitCode int, exitMessage string) {
	return routingTableMatch(7, parameters[0])
}

// routeTableDestination checks if an IP address is a gateway's IP in the
// kernel's IP routing table, as accessed by `route -n`.
func RoutingTableGateway(parameters []string) (exitCode int, exitMessage string) {
	return routingTableMatch(1, parameters[0])
}
