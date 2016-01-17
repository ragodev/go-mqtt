package client

import (
	"bufio"
	"errors"
	"net"
	"net/url"

	"github.com/koron/go-mqtt/packet"
)

// Param represents connection parameters for MQTT client.
type Param struct {
	// Addr is URL to connect like "tcp://192.168.0.1:1883".
	Addr string

	// ID is used as MQTT's client ID.
	ID string

	// Options is option parameters for connection.
	Options *Options
}

func (p *Param) options() *Options {
	if p.Options == nil {
		return DefaultOptions
	}
	return p.Options
}

func (p *Param) addr() string {
	if p.Addr == "" {
		return "tcp://127.0.0.1:1883"
	}
	return p.Addr
}

func (p *Param) url() (*url.URL, error) {
	return url.Parse(p.addr())
}

func (p *Param) connectPacket() *packet.Connect {
	return p.options().connectPacket(p.ID)
}

func (p *Param) newReadWriter(c net.Conn) *bufio.ReadWriter {
	// TODO: apply timeout configuration.
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	return bufio.NewReadWriter(r, w)
}

// Options represents connect options
type Options struct {
	Version      uint8   // MQTT's protocol version 3 or 4 (fallback to 4)
	Username     *string // username to connect (option)
	Password     *string // password to connect (option)
	CleanSession bool
	KeepAlive    uint16
	Will         *Will
}

func (o *Options) version() uint8 {
	switch o.Version {
	case 3:
		return 3
	default:
		return 4
	}
}

func (o *Options) connectPacket(id string) *packet.Connect {
	p := &packet.Connect{
		ClientID:     id,
		Version:      o.version(),
		Username:     o.Username,
		Password:     o.Password,
		CleanSession: o.CleanSession,
		KeepAlive:    o.KeepAlive,
	}
	if o.Will != nil {
		p.WillFlag = true
		p.WillQoS = o.Will.QoS.qos()
		p.WillRetain = o.Will.Retain
		p.WillTopic = o.Will.Topic
		p.WillMessage = o.Will.Message
	}
	return p
}

// DefaultOptions represents default values which used for when Connect()'s
// opts argument is nil.
var DefaultOptions = &Options{
	Version:      4,
	CleanSession: true,
	KeepAlive:    60,
}

// Will represents MQTT's will message.
type Will struct {
	QoS     QoS
	Retain  bool
	Topic   string
	Message string
}

// Reason represents reason of disconnection.
type Reason int

const (
	// Explicitly shows called Disconnect() explicitly.
	Explicitly Reason = iota

	// Timeout shows by timeout.
	Timeout
)

func (r Reason) Error() string {
	switch r {
	case Explicitly:
		return "disconnected explicitly"
	case Timeout:
		return "detect timeout"
	default:
		return "unknown reason"
	}
}

// DisconnectedFunc is called when a connection was lost.
// reason can be one of Reason or other errors.
type DisconnectedFunc func(reason error, param Param)

// Connect connects to MQTT broker and returns a Client.
func Connect(p Param, df DisconnectedFunc) (Client, error) {
	u, err := p.url()
	if err != nil {
		return nil, err
	}
	c, err := net.Dial(u.Scheme, u.Host)
	if err != nil {
		return nil, err
	}
	rw := p.newReadWriter(c)

	// send CONNECT packet.
	bc, err := p.connectPacket().Encode()
	if err != nil {
		c.Close()
		return nil, err
	}
	_, err = c.Write(bc)
	if err != nil {
		c.Close()
		return nil, err
	}

	// receive CONNACK packet.
	rp, err := packet.SplitDecode(rw)
	if err != nil {
		c.Close()
		return nil, err
	}
	ack, ok := rp.(*packet.ConnACK)
	if !ok {
		c.Close()
		return nil, errors.New("received non CONNACK")
	}
	if ack.ReturnCode != packet.ConnectAccept {
		return nil, ack.ReturnCode
	}

	cl := &client{
		conn: c,
		rw:   rw,
	}
	cl.start()
	return cl, nil
}
