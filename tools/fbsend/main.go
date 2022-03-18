package main

import (
	"context"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"flag"
	"fmt"
	"google.golang.org/api/option"
	"os"
	"strings"
)

func main() {
	conffile := flag.String("config", "/etc/fbsend/fbsend.json", "config file")
	flag.Parse()
	if flag.NArg() < 2 {
		fail("Syntax: fbsend [-config FILE] topic key=value ...")
	}
	topic := flag.Arg(0)
	data := make(map[string]string)
	for i := 1; i < flag.NArg(); i++ {
		kv := strings.SplitN(flag.Arg(i), "=", 2)
		if len(kv) != 2 {
			fail(fmt.Sprintf("Invalid argument: %s (%v)", flag.Arg(i), kv))
		}
		data[kv[0]] = kv[1]
	}
	fb, err := firebase.NewApp(context.Background(), nil, option.WithCredentialsFile(*conffile))
	if err != nil {
		fail(err.Error())
	}
	msg, err := fb.Messaging(context.Background())
	if err != nil {
		fail(err.Error())
	}
	_, err = msg.Send(context.Background(), &messaging.Message{
		Topic: topic,
		Data:  data,
	})
	if err != nil {
		fail(err.Error())
	}
	fmt.Println("Sent successfully")
}

func fail(s string) {
	fmt.Println(s)
	os.Exit(1)
}
