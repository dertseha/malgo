package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/go-mp3"
	"github.com/youpy/go-wav"

	"github.com/gen2brain/malgo"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("No input file.")
		os.Exit(1)
	}

	file, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer file.Close()

	var reader io.Reader
	var channels, sampleRate uint32

	switch strings.ToLower(filepath.Ext(os.Args[1])) {
	case ".wav":
		w := wav.NewReader(file)
		f, err := w.Format()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		reader = w
		channels = uint32(f.NumChannels)
		sampleRate = f.SampleRate

	case ".mp3":
		m, err := mp3.NewDecoder(file)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		reader = m
		channels = 2
		sampleRate = uint32(m.SampleRate())
	default:
		fmt.Println("Not a valid file.")
		os.Exit(1)
	}

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
		fmt.Printf("LOG <%v>\n", message)
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	deviceConfig := malgo.DefaultDeviceConfig()
	deviceConfig.Format = malgo.FormatS16
	deviceConfig.Channels = channels
	deviceConfig.SampleRate = sampleRate
	deviceConfig.Alsa.NoMMap = 1

	sampleSize := uint32(malgo.SampleSizeInBytes(deviceConfig.Format))
	// This is the function that's used for sending more data to the device for playback.
	onSendSamples := func(frameCount uint32, samples []byte) uint32 {
		n, _ := io.ReadFull(reader, samples)
		return uint32(n) / uint32(channels) / sampleSize
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Send: onSendSamples,
	}
	device, err := malgo.InitDevice(ctx.Context, malgo.Playback, nil, deviceConfig, deviceCallbacks)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer device.Uninit()

	err = device.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println("Press Enter to quit...")
	fmt.Scanln()
}
