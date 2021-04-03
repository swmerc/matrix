package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

// BrokerConfig is the section of a config file that describes how to connect to the broker(s)
type BrokerConfig struct {
	Name     string `yaml:"name"`
	Client   string `yaml:"client"`
	Host     string `yaml:"host"`
	Port     uint32 `yaml:"port"`
	TLS      bool   `yaml:"tls"`
	Username string `yaml:"user"`
	Password string `yaml:"pass"`
}

// BrokerMux needs a comment
type BrokerMux interface {
	Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token
	Subscribe(topic string, qos byte, callback func(client mqtt.Client, msg mqtt.Message)) mqtt.Token
}

//
// Init
//
func newBrokerMux(cfg []BrokerConfig) BrokerMux {
	bmux := &brokerMuxImpl{brokers: make(map[string]mqtt.Client)}

	for _, broker := range cfg {
		opts := mqtt.NewClientOptions()
		opts.CleanSession = false

		opts.SetClientID(broker.Client)

		if broker.TLS {
			tlsConfig := &tls.Config{InsecureSkipVerify: false, ClientAuth: tls.NoClientCert}
			opts.SetTLSConfig(tlsConfig)
			opts.AddBroker(fmt.Sprintf("ssl://%s:%d", broker.Host, broker.Port))
		} else {
			opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker.Host, broker.Port))
		}

		if len(broker.Username) > 0 && len(broker.Password) > 0 {
			opts.SetUsername(broker.Username)
			opts.SetPassword(broker.Password)
		}

		opts.OnConnect = connectHandler
		opts.OnConnectionLost = connectLostHandler

		//
		// We block if any of the brokers are down.  They can bounce all they want after we connect, we can't
		// start until they are up.  Tnis is not the end of the world since it is not like we can do a ton
		// without a network.  The only downside is that we hang here if we have a misconfigured MQTT
		// broker.
		//
		client := mqtt.NewClient(opts)
		for true {
			if token := client.Connect(); token.Wait() && token.Error() != nil {
				log.Infof("bmux: error connecting to broker %s at start: %s", broker.Name, token.Error())
				time.Sleep(time.Duration(1) * time.Minute)
			} else {
				break
			}
		}

		log.Infof("bmux: new broker: %s=%s:%d", broker.Name, broker.Host, broker.Port)
		bmux.brokers[broker.Name] = client
	}

	return bmux
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	r := client.OptionsReader()
	log.Infof("MQTT: connected: %s", r.ClientID())
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	r := client.OptionsReader()
	log.Infof("MQTT: disconnected: %s: %v", r.ClientID(), err)
}

//
// Implementation
//
type brokerMuxImpl struct {
	brokers map[string]mqtt.Client
}

func (bmux *brokerMuxImpl) Publish(muxTopic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	broker, topic := splitMuxTopic(muxTopic)
	if client, ok := bmux.brokers[broker]; ok {
		return client.Publish(topic, qos, retained, payload)
	}
	return newErrorToken(broker + " is not a valid broker")
}

func (bmux *brokerMuxImpl) Subscribe(muxTopic string, qos byte, callback func(client mqtt.Client, msg mqtt.Message)) mqtt.Token {
	broker, topic := splitMuxTopic(muxTopic)
	if client, ok := bmux.brokers[broker]; ok {
		log.Debugf("bmux: subscribe: %s:%s", broker, topic)
		return client.Subscribe(topic, qos, func(client mqtt.Client, msg mqtt.Message) {
			wrappedMsg := &mqttMessageWrapper{msg: msg, topic: broker + ":" + msg.Topic()}
			callback(client, wrappedMsg)
		})
	}
	return newErrorToken(broker + " is not a valid broker")
}

func splitMuxTopic(muxTopic string) (string, string) {
	ret := strings.SplitN(muxTopic, ":", 2)
	if len(ret) < 2 {
		return "UNKNOWN", ""
	}
	return ret[0], ret[1]
}

//
//  Sigh.  I need to implement mqtt.Message so I can fix the topic.  There has to be a cleaner way to do this.
//
type mqttMessageWrapper struct {
	msg   mqtt.Message
	topic string
}

func (m *mqttMessageWrapper) Duplicate() bool {
	return m.msg.Duplicate()
}

func (m *mqttMessageWrapper) Qos() byte {
	return m.msg.Qos()
}

func (m *mqttMessageWrapper) Retained() bool {
	return m.msg.Retained()
}

func (m *mqttMessageWrapper) Topic() string {
	return m.topic
}

func (m *mqttMessageWrapper) MessageID() uint16 {
	return m.msg.MessageID()
}

func (m *mqttMessageWrapper) Payload() []byte {
	return m.msg.Payload()
}

func (m *mqttMessageWrapper) Ack() {
	m.msg.Ack()
}

//
// Sigh #2: This is the only way to return errors from my mux'd brokers in the case
// where a given broker does not exist.
//
// It should work like a real mqtt.Token(), I guess.
//
type errorToken struct {
	complete chan struct{}
	err      error
}

func (e *errorToken) Wait() bool {
	return true
}

func (e *errorToken) WaitTimeout(time.Duration) bool {
	return true
}

func (e *errorToken) Error() error {
	return e.err
}

func (e *errorToken) Done() <-chan struct{} {
	return e.complete
}

func newErrorToken(text string) mqtt.Token {
	e := &errorToken{complete: make(chan struct{}), err: errors.New(text)}
	close(e.complete)
	return e
}
