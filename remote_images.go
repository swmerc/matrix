package main

import (
	"image"
	"net/http"
	"strconv"
	"time"

	"github.com/disintegration/imaging"
	log "github.com/sirupsen/logrus"
)

// RemoteImageConfig supports cameras or other changing images available via HTTP
type RemoteImageConfig struct {
	Topic   string `yaml:"topic"`
	Height  int    `yaml:"height"`
	Width   int    `yaml:"width"`
	Sources []struct {
		URI          string       `yaml:"uri"`
		Jobs         JobRunnerCfg `yaml:"jobs"`
		ResizeWidth  int          `yaml:"resizeWidth"`
		ResizeHeight int          `yaml:"resizeHeight"`
		StartX       int          `yaml:"startX"`
		StartY       int          `yaml:"startY"`
	} `yaml:"sources"`
}

// FIXME: I eventually want to share the processing chain between local and remote, with the only
//        real difference being the source of the content.  Local images are currently all
//        the proper size, but whatever.
type croppedImage struct {
	resizeHeight int
	resizeWidth  int
	startX       int
	startY       int
	height       int
	width        int
}

func initRemoteImages(bmux BrokerMux, cfg RemoteImageConfig) {
	if len(cfg.Topic) <= 0 {
		return
	}

	for sourceIndex, source := range cfg.Sources {
		remoteSource := source
		NewJobRunner("images-remote-"+strconv.Itoa(sourceIndex), source.Jobs, func() {
			img := &croppedImage{
				resizeHeight: remoteSource.ResizeHeight,
				resizeWidth:  remoteSource.ResizeWidth,
				startX:       remoteSource.StartX,
				startY:       remoteSource.StartY,
				height:       cfg.Height,
				width:        cfg.Width,
			}
			publishRemoteImage(bmux, cfg.Topic, remoteSource.URI, img)
		}).Run()
	}
}

//
// This is where all of the magic happens
//
func publishRemoteImage(bmux BrokerMux, topic string, uri string, img *croppedImage) {
	// Read the data
	var myClient = &http.Client{Timeout: 15 * time.Second}

	rawResp, err := myClient.Get(uri)
	if err != nil {
		log.Errorf("remoteImage: GET error: %s: %s", uri, err.Error())
		return
	}
	defer rawResp.Body.Close()

	if rawResp.StatusCode != http.StatusOK {
		log.Errorf("remoteImage: GET bad response: %s: %d", uri, rawResp.StatusCode)
		return
	}

	// Turn it into an image
	orig, err := imaging.Decode(rawResp.Body)
	if err != nil {
		log.Errorf("remoteImage: bad decode: %s: %v", uri, err)
		return
	}

	// Resize
	scaled := imaging.Resize(orig, img.resizeWidth, img.resizeHeight, imaging.Lanczos)

	// Crop
	rect := image.Rectangle{
		Min: image.Pt(img.startX, img.startY),
		Max: image.Pt((img.startX + img.width), (img.startY + img.height))}
	cropped := imaging.Crop(scaled, rect)

	// Sanity check
	finalSize := cropped.Bounds()
	if finalSize.Max.X != img.width || finalSize.Max.Y != img.height {
		log.Errorf("remoteImage: borked: %s: %v", uri, finalSize)
	}

	// Publish whatever we have.
	log.Infof("remoteImage: posting to %s", topic)
	bmux.Publish(topic, 0, false, ImageToMatrixBytes(cropped))
}
