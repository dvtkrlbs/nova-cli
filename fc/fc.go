package fc

import (
	"errors"
	"github.com/gtu-nova/nova-cli/msp"
	"github.com/sirupsen/logrus"
	"io"
	"time"
)

type MspCallback func(fr msp.Frame, fc *FC) error

// FC represents a connection to the flight controller, which can
// handle disconnections and reconnections on its on. Use NewFC()
// to initialize an FC and then call FC.mainLoop().
type FC struct {
	msp            *msp.MSP
	callbackMap    map[uint16]MspCallback
	onFrame        MspCallback
	logger         *logrus.Logger
	closeChan      chan bool
	isReconnecting bool
	isRebooting    bool
}

type ReadWriteReconnect interface {
	io.ReadWriter
	Reconnect() error
}

// NewFC returns a new FC using the given port and baud rate. stdout is
// optional and will default to os.Stdout if nil
func NewFC(port ReadWriteReconnect, frCallback MspCallback, logger *logrus.Logger) (*FC, error) {
	m, err := msp.New(port, logger)
	if err != nil {
		return nil, err
	}
	fc := &FC{
		msp:         m,
		callbackMap: make(map[uint16]MspCallback),
		logger:      logger,
		onFrame:     frCallback,
		closeChan:   make(chan bool),
	}
	fc.mainLoop()
	return fc, nil
}

func (f *FC) AddCallback(msgId uint16, fn MspCallback) {
	f.callbackMap[msgId] = fn
}

func (f *FC) AddCallbacks(msgIds []uint16, fns []MspCallback) {
	if len(msgIds) != len(fns) {
		panic("The ids slice and the functions slice are not equal")
	}

	for i, id := range msgIds {
		f.AddCallback(id, fns[i])
	}
}

func (f *FC) Reconnect() {
	f.isReconnecting = true
	p := f.msp.Port.(ReadWriteReconnect)
	if err := p.Reconnect(); err != nil {
		f.logger.Fatal("Failed to reconnect (%v)", err)
	}
	f.logger.Info("Reconnected to the flight controller")
	f.isReconnecting = false
}

func (f *FC) WriteCmd(cmd uint16, args ...interface{}) (int, error) {
	if !f.isReconnecting && !f.isRebooting {
		return f.msp.WriteCmd(cmd, args...)
	} else {
		return 0, nil
	}
}

func (f *FC) Close() {
	f.closeChan <- true
}

func (f *FC) mainLoop() {

	ticker := time.NewTicker(10 * time.Millisecond)

	go func() {
		defer f.logger.Info("Main loop ended")
		for {
			select {
			case _ = <-ticker.C:
				var frame *msp.Frame
				var err error

				if !f.isReconnecting {
					frame, err = f.msp.ReadFrame()

					if err != nil {
						var perr *msp.InvalidPacketError
						if errors.As(err, &perr) {
							f.logger.Warnf("Invalid packet (%v)\n", err)
							continue
						} else {
							f.logger.Warnf("Attempting to reconnect")
							f.Reconnect()
						}
					}
					if frame != nil {
						if callback, found := f.callbackMap[frame.Code]; found {
							err = callback(*frame, f)
						} else {
							err = f.onFrame(*frame, f)
						}
						if err != nil {
							f.logger.Errorf("Error in callback for message code %d (%v)\n", frame.Code, err)
						}

					}
				}
			case <-f.closeChan:
				return
			}
		}
	}()
}
