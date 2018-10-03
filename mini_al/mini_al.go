// Package mal - Mini audio library (mini_al cgo bindings).
package mini_al

/*
#cgo CFLAGS: -std=gnu99

#cgo linux LDFLAGS: -ldl -lpthread -lm
#cgo openbsd LDFLAGS: -lpthread -lm -lossaudio
#cgo netbsd LDFLAGS: -lpthread -lm -lossaudio
#cgo freebsd LDFLAGS: -lpthread -lm
#cgo android LDFLAGS: -lOpenSLES

#cgo !noasm,!arm,!arm64 CFLAGS: -msse2
#cgo !noasm,arm,arm64 CFLAGS: -mfpu=neon -mfloat-abi=hard
#cgo noasm CFLAGS: -DMAL_NO_SSE2 -DMAL_NO_AVX -DMAL_NO_AVX512 -DMAL_NO_NEON

#include "malgo.h"
*/
import "C"

import (
	"fmt"
	"reflect"
	"unsafe"
)

// Device type.
type Device struct {
	context *C.mal_context
	device  *C.mal_device
}

// NewDevice returns new Device.
func NewDevice() *Device {
	d := &Device{}
	d.context = C.goGetContext()
	d.device = C.goGetDevice()
	return d
}

// DeviceID type.
type DeviceID [unsafe.Sizeof(C.mal_device_id{})]byte

// cptr return C pointer.
func (d *DeviceID) cptr() *C.mal_device_id {
	return (*C.mal_device_id)(unsafe.Pointer(d))
}

// DeviceInfo type.
type DeviceInfo struct {
	ID            DeviceID
	Name          [256]byte
	FormatCount   uint32
	Formats       [6]uint32
	MinChannels   uint32
	MaxChannels   uint32
	MinSampleRate uint32
	MaxSampleRate uint32
}

// String returns string.
func (d *DeviceInfo) String() string {
	return fmt.Sprintf("{ID: %s, Name: %s}", string(d.ID[:]), string(d.Name[:]))
}

func deviceInfoFromPointer(ptr unsafe.Pointer) DeviceInfo {
	return *(*DeviceInfo)(ptr)
}

// AlsaDeviceConfig type.
type AlsaDeviceConfig struct {
	NoMMap uint32
}

// PulseDeviceConfig type.
type PulseDeviceConfig struct {
	StreamName *byte
}

// DeviceConfig type.
type DeviceConfig struct {
	Format             FormatType
	Channels           uint32
	SampleRate         uint32
	ChannelMap         [32]byte
	BufferSizeInFrames uint32
	Periods            uint32
	ShareMode          ShareMode
	PerformanceProfile PerformanceProfile
	_                  [4]byte
	OnRecvCallback     *[0]byte
	OnSendCallback     *[0]byte
	OnStopCallback     *[0]byte
	Alsa               AlsaDeviceConfig
	_                  [4]byte
	Pulse              PulseDeviceConfig
}

// cptr return C pointer.
func (d *DeviceConfig) cptr() *C.mal_device_config {
	return (*C.mal_device_config)(unsafe.Pointer(d))
}

func deviceConfigFromPointer(ptr unsafe.Pointer) DeviceConfig {
	return *(*DeviceConfig)(ptr)
}

// RecvProc type.
type RecvProc func(framecount uint32, psamples []byte)

// SendProc type.
type SendProc func(framecount uint32, psamples []byte) uint32

// StopProc type.
type StopProc func()

// Handlers.
var (
	recvHandler RecvProc
	sendHandler SendProc
	stopHandler StopProc
	logHandler  LogProc
)

//export goRecvCallback
func goRecvCallback(pDevice *C.mal_device, frameCount C.mal_uint32, pSamples unsafe.Pointer) {
	if recvHandler != nil {
		sampleCount := uint32(frameCount) * uint32(pDevice.channels)
		sizeInBytes := uint32(C.mal_get_bytes_per_sample(pDevice.format))
		psamples := (*[1 << 20]byte)(pSamples)[0 : sampleCount*sizeInBytes]
		recvHandler(uint32(frameCount), psamples)
	}
}

//export goSendCallback
func goSendCallback(pDevice *C.mal_device, frameCount C.mal_uint32, pSamples unsafe.Pointer) (r C.mal_uint32) {
	if sendHandler != nil {
		sampleCount := uint32(frameCount) * uint32(pDevice.channels)
		sizeInBytes := uint32(C.mal_get_bytes_per_sample(pDevice.format))
		psamples := (*[1 << 20]byte)(pSamples)[0 : sampleCount*sizeInBytes]
		r = C.mal_uint32(sendHandler(uint32(frameCount), psamples))
	}
	return r
}

//export goStopCallback
func goStopCallback(pDevice *C.mal_device) {
	if stopHandler != nil {
		stopHandler()
	}
}

// ContextInit initializes a context.
//
// The context is used for selecting and initializing the relevant backends.
//
// <backends> is used to allow the application to prioritize backends depending on it's specific
// requirements. This can be nil in which case it uses the default priority, which is as follows:
//   - WASAPI
//   - DirectSound
//   - WinMM
//   - ALSA
//   - OSS
//   - OpenSL|ES
//   - OpenAL
//   - Null
//
// This will dynamically load backends DLLs/SOs (such as dsound.dll).
func (d *Device) ContextInit(backends []Backend, config ContextConfig) error {
	cbackends := (*C.mal_backend)(unsafe.Pointer((*reflect.SliceHeader)(unsafe.Pointer(&backends)).Data))
	cbackendcount := (C.mal_uint32)(len(backends))
	cconfig := config.cptr()

	ret := C.mal_context_init(cbackends, cbackendcount, cconfig, d.context)
	v := (Result)(ret)
	return errorFromResult(v)
}

// ContextUninit uninitializes a context.
//
// This will unload the backend DLLs/SOs.
func (d *Device) ContextUninit() error {
	ret := C.mal_context_uninit(d.context)
	v := (Result)(ret)
	return errorFromResult(v)
}

// Devices retrieves basic information about every active playback or capture device.
func (d *Device) Devices(kind DeviceType) ([]DeviceInfo, error) {
	var pcount uint32
	var ccount uint32

	pinfo := make([]*C.mal_device_info, 32)
	cinfo := make([]*C.mal_device_info, 32)

	cpcount := (*C.mal_uint32)(unsafe.Pointer(&pcount))
	cccount := (*C.mal_uint32)(unsafe.Pointer(&ccount))

	cpinfo := (**C.mal_device_info)(unsafe.Pointer(&pinfo[0]))
	ccinfo := (**C.mal_device_info)(unsafe.Pointer(&cinfo[0]))

	ret := C.mal_context_get_devices(d.context, cpinfo, cpcount, ccinfo, cccount)
	v := (Result)(ret)

	if v == Success {
		res := make([]DeviceInfo, 0)

		if kind == Playback {
			tmp := (*[1 << 20]*C.mal_device_info)(unsafe.Pointer(cpinfo))[:pcount]
			for _, d := range tmp {
				if d != nil {
					res = append(res, deviceInfoFromPointer(unsafe.Pointer(d)))
				}
			}
		} else if kind == Capture {
			tmp := (*[1 << 20]*C.mal_device_info)(unsafe.Pointer(ccinfo))[:ccount]
			for _, d := range tmp {
				if d != nil {
					res = append(res, deviceInfoFromPointer(unsafe.Pointer(d)))
				}
			}
		}

		return res, nil
	}

	return nil, errorFromResult(v)
}

// Init initializes a device.
//
// The device ID (pdeviceid) can be nil, in which case the default device is used. Otherwise, you
// can retrieve the ID by calling EnumerateDevices() and use the ID from the returned data.
//
// Set pdeviceid to nil to use the default device. Do _not_ rely on the first device ID returned
// by EnumerateDevices() to be the default device.
//
// Consider using ConfigInit(), ConfigInitPlayback(), etc. to make it easier
// to initialize a DeviceConfig object.
func (d *Device) Init(kind DeviceType, pdeviceid *DeviceID, pconfig *DeviceConfig) error {
	ckind := (C.mal_device_type)(kind)
	cpdeviceid := pdeviceid.cptr()
	cpconfig := pconfig.cptr()

	ret := C.mal_device_init(d.context, ckind, cpdeviceid, cpconfig, nil, d.device)
	v := (Result)(ret)
	return errorFromResult(v)
}

// Uninit uninitializes a device.
//
// This will explicitly stop the device. You do not need to call Stop() beforehand, but it's harmless if you do.
func (d *Device) Uninit() {
	C.mal_device_uninit(d.device)
}

// SetRecvCallback sets the callback to use when the application has received data from the device.
func (d *Device) SetRecvCallback(proc RecvProc) {
	recvHandler = proc
	C.goSetRecvCallback(d.device)
}

// SetSendCallback sets the callback to use when the application needs to send data to the device for playback.
func (d *Device) SetSendCallback(proc SendProc) {
	sendHandler = proc
	C.goSetSendCallback(d.device)
}

// SetStopCallback sets the callback to use when the device has stopped, either explicitly or as a result of an error.
func (d *Device) SetStopCallback(proc StopProc) {
	stopHandler = proc
	C.goSetStopCallback(d.device)
}

// SetLogCallback sets the log callback.
func (d *Device) SetLogCallback(proc LogProc) {
	logHandler = proc
}

// Start activates the device. For playback devices this begins playback. For capture devices it begins recording.
//
// For a playback device, this will retrieve an initial chunk of audio data from the client before
// returning. The reason for this is to ensure there is valid audio data in the buffer, which needs
// to be done _before_ the device begins playback.
func (d *Device) Start() error {
	ret := C.mal_device_start(d.device)
	v := (Result)(ret)
	return errorFromResult(v)
}

// Stop puts the device to sleep, but does not uninitialize it. Use Start() to start it up again.
func (d *Device) Stop() error {
	ret := C.mal_device_stop(d.device)
	v := (Result)(ret)
	return errorFromResult(v)
}

// IsStarted determines whether or not the device is started.
func (d *Device) IsStarted() (r bool) {
	ret := C.mal_device_is_started(d.device)
	v := (uint32)(ret)
	if v == 1 {
		r = true
	}
	return r
}

// ConfigInit is a helper function for initializing a DeviceConfig object.
func (d *Device) ConfigInit(format FormatType, channels uint32, samplerate uint32, onrecvcallback RecvProc, onsendcallback SendProc) DeviceConfig {
	cformat := (C.mal_format)(format)
	cchannels := (C.mal_uint32)(channels)
	csamplerate := (C.mal_uint32)(samplerate)

	recvHandler = onrecvcallback
	sendHandler = onsendcallback

	ret := C.goConfigInit(cformat, cchannels, csamplerate)
	v := deviceConfigFromPointer(unsafe.Pointer(&ret))
	return v
}

// ConfigInitCapture is a simplified version of DeviceConfigInit() for capture devices.
func (d *Device) ConfigInitCapture(format FormatType, channels uint32, samplerate uint32, onrecvcallback RecvProc) DeviceConfig {
	cformat := (C.mal_format)(format)
	cchannels := (C.mal_uint32)(channels)
	csamplerate := (C.mal_uint32)(samplerate)

	recvHandler = onrecvcallback

	ret := C.goConfigInitCapture(cformat, cchannels, csamplerate)
	v := deviceConfigFromPointer(unsafe.Pointer(&ret))
	return v
}

// ConfigInitPlayback is a simplified version of DeviceConfigInit() for playback devices.
func (d *Device) ConfigInitPlayback(format FormatType, channels uint32, samplerate uint32, onsendcallback SendProc) DeviceConfig {
	cformat := (C.mal_format)(format)
	cchannels := (C.mal_uint32)(channels)
	csamplerate := (C.mal_uint32)(samplerate)

	sendHandler = onsendcallback

	ret := C.goConfigInitPlayback(cformat, cchannels, csamplerate)
	v := deviceConfigFromPointer(unsafe.Pointer(&ret))
	return v
}

// ConfigInitDefaultCapture initializes a default capture device config.
func (d *Device) ConfigInitDefaultCapture(onrecvcallback RecvProc) DeviceConfig {
	recvHandler = onrecvcallback

	ret := C.goConfigInitDefaultCapture()
	v := deviceConfigFromPointer(unsafe.Pointer(&ret))
	return v
}

// ConfigInitDefaultPlayback initializes a default playback device config.
func (d *Device) ConfigInitDefaultPlayback(onsendcallback SendProc) DeviceConfig {
	sendHandler = onsendcallback

	ret := C.goConfigInitDefaultPlayback()
	v := deviceConfigFromPointer(unsafe.Pointer(&ret))
	return v
}

// ContextConfigInit is a helper function for initializing a ContextConfig object.
func (d *Device) ContextConfigInit(onlogcallback LogProc) ContextConfig {
	logHandler = onlogcallback

	ret := C.goContextConfigInit()
	v := contextConfigFromPointer(unsafe.Pointer(&ret))
	return v
}

// BufferSizeInBytes retrieves the size of the buffer in bytes.
func (d *Device) BufferSizeInBytes() uint32 {
	ret := C.mal_device_get_buffer_size_in_bytes(d.device)
	v := (uint32)(ret)
	return v
}

// SampleSizeInBytes retrieves the size of a sample in bytes for the given format.
func (d *Device) SampleSizeInBytes(format FormatType) uint32 {
	cformat := (C.mal_format)(format)
	ret := C.mal_get_bytes_per_sample(cformat)
	v := (uint32)(ret)
	return v
}

// Type returns device type.
func (d *Device) Type() DeviceType {
	return DeviceType(d.device._type)
}

// Format returns device format.
func (d *Device) Format() FormatType {
	return FormatType(d.device.format)
}

// Channels returns number of channels.
func (d *Device) Channels() uint32 {
	return uint32(d.device.channels)
}

// SampleRate returns sample rate.
func (d *Device) SampleRate() uint32 {
	return uint32(d.device.sampleRate)
}