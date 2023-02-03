package server

import (
    "fmt"
	"bytes"
    mqtt "github.com/eclipse/paho.mqtt.golang"
	"heckel.io/ntfy/log"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
)

type mqttBackend struct {
	config  *Config
	handler func(http.ResponseWriter, *http.Request)
	client  mqtt.Client
}

type mqttMessage struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Priority string   `json:"priority"`
	Tags     string   `json:"tags"`
	Actions  string   `json:"actions"`
}

func newMqttBackend(conf *Config, handler func(http.ResponseWriter, *http.Request)) *mqttBackend {
	return &mqttBackend{
		config:  conf,
		handler: handler,
	}
}

func (b *mqttBackend) Connect() () {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", b.config.MqttServer, b.config.MqttPort))
    opts.SetClientID("ntfy")
    opts.SetUsername(b.config.MqttUsername)
    opts.SetPassword(b.config.MqttPassword)
	log.Info("[mqtt] Connect to mqtt server %s:%d", b.config.MqttServer, b.config.MqttPort)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Debug("[mqtt] Can not connect")
        return
    }
	b.client = client
	return
}

func (b *mqttBackend) Subscribe() () {
	b.client.Subscribe(b.config.MqttTopic+"/message", 0, func(client mqtt.Client, msg mqtt.Message) {
		log.Debug("[mqtt] Received message [%s] %s ", msg.Topic(), string(msg.Payload()))
		var m mqttMessage
		if err := json.NewDecoder(bytes.NewReader(msg.Payload())).Decode(&m); err != nil {
			log.Info("[mqtt] not a valide json")
			return
		}
		if !topicRegex.MatchString(m.Topic) {
			log.Info("[mqtt] not a valide topic")
			return
		}
		if m.Message == "" {
			m.Message = emptyMessageBody
		}
		url := fmt.Sprintf("%s/%s", b.config.BaseURL, m.Topic)
		req, err := http.NewRequest("POST", url, strings.NewReader(m.Message))
		req.RequestURI = "/" + m.Topic 
		req.RemoteAddr = "127.0.0.1"
		if err != nil {
			return
		}
		if m.Title != "" {
			req.Header.Set("Title", m.Title)
		}
		if m.Priority != "" {
			req.Header.Set("Priority",m.Priority)
		}
		if m.Tags != "" {
			req.Header.Set("Tags",m.Tags)
		}
		if m.Actions != "" {
			req.Header.Set("Actions",m.Actions)
		}
		rr := httptest.NewRecorder()
		b.handler(rr, req)
		if rr.Code != http.StatusOK {
			return
		}
	})
	log.Info("[mqtt] Subscribed to topic %s/message", b.config.MqttTopic)
}