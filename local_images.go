package main

import (
	"image"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/disintegration/imaging"
)

// LocalImagesConfig supports a single dest and a list of image to randomly publish to that topic
type LocalImagesConfig struct {
	Topic      string   `yaml:"topic"`
	Height     int      `yaml:"height"`
	Width      int      `yaml:"width"`
	FixedDelay int      `yaml:"fixedDelay"`
	RandDelay  int      `yaml:"randDelay"`
	Sources    []string `yaml:"sources"`
}

type localImageImpl struct {
	bmux BrokerMux
	cfg  LocalImagesConfig
}

func initLocalImages(bmux BrokerMux, cfg LocalImagesConfig) {
	if len(cfg.Topic) > 0 && len(cfg.Sources) > 0 {
		i := &localImageImpl{bmux: bmux, cfg: cfg}
		go i.loop()
	}
}

// ImageToMatrixBytes turns a NRGBA image into an array of 3 byte pixels
func ImageToMatrixBytes(img *image.NRGBA) []byte {

	maxX := img.Bounds().Max.X
	maxY := img.Bounds().Max.Y

	// Create the raw byte output.  Hope the RGBs line up
	output := make([]byte, maxX*maxY*3)
	index := 0
	for y := 0; y < maxY; y++ {
		for x := 0; x < maxX; x++ {
			rt, gt, bt, _ := img.At(x, y).RGBA()
			output[index+0] = byte(rt)
			output[index+1] = byte(gt)
			output[index+2] = byte(bt)
			index = index + 3
		}
	}
	return output
}

func (i *localImageImpl) loop() {

	for true {
		// Delay until the next time
		delay := i.cfg.FixedDelay

		if i.cfg.RandDelay > 0 {
			delay = delay + rand.Intn(i.cfg.RandDelay)
		}

		if delay < 0 {
			delay = 2
		}

		log.Debugf("images: sleeping %d minutes", delay)
		time.Sleep(time.Duration(delay) * time.Minute)

		//
		// Pick a random image
		//
		index := 0
		if len(i.cfg.Sources) > 1 {
			index = rand.Intn(len(i.cfg.Sources))
		}

		//
		// Process it
		//
		img, err := imaging.Open(i.cfg.Sources[index])
		if err != nil {
			log.Errorf("images: %s: open: %v", i.cfg.Sources[index], err)
			continue
		}

		final := imaging.Resize(img, i.cfg.Width, i.cfg.Height, imaging.Lanczos)
		if err != nil {
			log.Errorf("images: %s: resize: %v", i.cfg.Sources[index], err)
			continue
		}

		log.Infof("images: posting %s to %s", i.cfg.Sources[index], i.cfg.Topic)
		i.bmux.Publish(i.cfg.Topic, 0, false, ImageToMatrixBytes(final))
	}

}
