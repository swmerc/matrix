package main

import (
	"fmt"
	"image"
	"net/http"
	"time"

	"github.com/disintegration/imaging"
	log "github.com/sirupsen/logrus"
)

// RemoteImageConfig supports remoteImages or other changing images available via HTTP
type RemoteImageConfig struct {
	Topic   string `yaml:"topic"`
	Height  int    `yaml:"height"`
	Width   int    `yaml:"width"`
	Sources []struct {
		URI          string `yaml:"uri"`
		Offsets      []int  `yaml:"offsets"`
		ResizeWidth  int    `yaml:"resizeWidth"`
		ResizeHeight int    `yaml:"resizeHeight"`
		StartX       int    `yaml:"startX"`
		StartY       int    `yaml:"startY"`
	} `yaml:"sources"`
}

//
// Job that is runnable via offsetJobRunner directly.  I could save some space and
// assume that all of the topics and URIs are the same, but I hope to extend this
// at some point (perhaps with different job runners for different topics).  It is
// not wasting a ton of space, so I'm not sure I care anyway.
//
type remoteImageJob struct {
	// Publish support
	bmux  BrokerMux
	topic string

	// Home of the image we want
	uri string

	// Resize to this
	resizeHeight int
	resizeWidth  int

	// then start grabbing data here
	startX int
	startY int

	// and limit to this output size
	outputHeight int
	outputWidth  int
}

func initRemoteImages(bmux BrokerMux, cfgs []RemoteImageConfig) {

	// Walk the config, making a remoteImageJob for each entry
	for idx, cfg := range cfgs {

		runner := newOffsetJobRunner(fmt.Sprintf("remote%d", idx))

		for _, source := range cfg.Sources {

			job := &remoteImageJob{
				bmux:         bmux,
				topic:        cfg.Topic,
				uri:          source.URI,
				resizeHeight: source.ResizeHeight,
				resizeWidth:  source.ResizeWidth,
				startX:       source.StartX,
				startY:       source.StartY,
				outputHeight: cfg.Height,
				outputWidth:  cfg.Width,
			}

			for _, offset := range source.Offsets {
				runner.AddJob(offset, job)
			}
		}

		runner.Run()
	}
}

//
// This is where all of the magic happens
//
func (c *remoteImageJob) Run() {
	// Read the data
	var myClient = &http.Client{Timeout: 15 * time.Second}

	rawResp, err := myClient.Get(c.uri)
	if err != nil {
		log.Errorf("remoteImage: GET error: %s: %s", c.uri, err.Error())
		return
	}
	defer rawResp.Body.Close()

	if rawResp.StatusCode != http.StatusOK {
		log.Errorf("remoteImage: GET bad response: %s: %d", c.uri, rawResp.StatusCode)
		return
	}

	// Turn it into an image
	orig, err := imaging.Decode(rawResp.Body)
	if err != nil {
		log.Errorf("remoteImage: bad decode: %s: %v", c.uri, err)
		return
	}

	// Resize
	scaled := imaging.Resize(orig, c.resizeWidth, c.resizeHeight, imaging.Lanczos)

	// Crop
	rect := image.Rectangle{Min: image.Pt(c.startX, c.startY), Max: image.Pt((c.startX + c.outputWidth), (c.startY + c.outputHeight))}
	cropped := imaging.Crop(scaled, rect)

	// Sanity check
	finalSize := cropped.Bounds()
	if finalSize.Max.X != c.outputWidth || finalSize.Max.Y != c.outputHeight {
		log.Errorf("remoteImage: borked: %s: %v", c.uri, finalSize)
	}

	// Publish whatever we have.
	log.Infof("remoteImage: posting to %s", c.topic)
	c.bmux.Publish(c.topic, 0, false, ImageToMatrixBytes(cropped))
}
