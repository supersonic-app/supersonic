package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/supersonic-app/go-upnpcast/device"
	"github.com/supersonic-app/go-upnpcast/services/avtransport"
)

func main() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var devices []*device.MediaRenderer
	var err error
	if devices, err = device.SearchMediaRenderers(context.Background(), 5); err != nil {
		log.Printf("Error loading devices: %v", err)
	}

	if len(devices) == 0 {
		log.Printf("No devices")
		return
	}
	dev := devices[0]
	cli, err := dev.AVTransportClient()
	if err != nil {
		panic(err)
	}
	err = cli.SetAVTransportMedia(context.Background(), &avtransport.MediaItem{
		URL:   `https://file-examples.com/storage/fe6a71582967c9a269c25cd/2017/11/file_example_MP3_700KB.mp3`,
		Title: "Foo",
	})
	cli.Play(context.Background())

	err = cli.SetNextAVTransportMedia(context.Background(), &avtransport.MediaItem{
		URL:   `https://download.samplelib.com/mp3/sample-15s.mp3`,
		Title: "Example mp3",
	})
	if err != nil {
		log.Println(err.Error())
	}
	log.Println("Playing two files")

	go func() {
		<-sigChan
		cli.Stop(context.Background())
		os.Exit(0)
	}()

	pos, _ := cli.GetPositionInfo(context.Background())
	log.Printf("%+v", pos)

	time.Sleep(3 * time.Second)

	cli.Seek(context.Background(), 35)
	pos, _ = cli.GetPositionInfo(context.Background())
	log.Printf("%+v", pos)
}
