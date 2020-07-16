package fc

import (
	"errors"
	"github.com/gtu-nova/nova-cli/msp"
	"github.com/sirupsen/logrus"
	"io"
)

type MspCallback func(fr msp.Frame, fc *FC)

// FC represents a connection to the flight controller, which can
// handle disconnections and reconnections on its on. Use NewFC()
// to initialize an FC and then call FC.mainLoop().
type FC struct {
	msp         *msp.MSP
	logger      *logrus.Logger
}

// NewFC returns a new FC using the given port and baud rate. stdout is
// optional and will default to os.Stdout if nil
func NewFC(port io.ReadWriter, logger *logrus.Logger) (*FC, error) {
	m, err := msp.New(port, logger)
	if err != nil {
		return nil, err
	}
	fc := &FC{
		msp:         m,
		logger:      logger,
	}
	//fc.mainLoop()
	return fc, nil
}


func (f *FC) WriteCmd(cmd uint16, args ...interface{}) (*msp.Frame, error) {
	_, err := f.msp.WriteCmd(cmd, args...)
	if err != nil {
		return nil, err
	}

	frame, err := f.msp.ReadFrame()

	if err != nil {
		var perr *msp.InvalidPacketError
		if errors.As(err, &perr) {
			f.logger.Warnf("Invalid packet (%v)\n", err)
			return nil, err
		} else {
			f.logger.Fatalf("Connection Lost")
		}
	}
	if frame != nil {
		return frame, nil
	}

	return nil, err
}