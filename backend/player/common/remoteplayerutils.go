package common

import (
	"sync/atomic"
	"time"
)

type TrackChangeTimer struct {
	timerActive atomic.Bool
	timer       *time.Timer
	resetChan   chan (time.Duration)

	onHandleTrackChange func()
}

func NewTrackChangeTimer(onHandleTrackChange func()) TrackChangeTimer {
	return TrackChangeTimer{
		resetChan:           make(chan time.Duration),
		onHandleTrackChange: onHandleTrackChange,
	}
}

func (d *TrackChangeTimer) Reset(dur time.Duration) {
	if d.timerActive.Swap(true) {
		// was active
		d.resetChan <- dur
		return
	}
	if dur == 0 {
		d.timerActive.Store(false)
		return
	}

	d.timer = time.NewTimer(dur)
	go func() {
		for {
			select {
			case dur := <-d.resetChan:
				if dur == 0 {
					d.timerActive.Store(false)
					if !d.timer.Stop() {
						select {
						case <-d.timer.C:
						default:
						}
					}
					d.timer = nil
					return
				}
				// reset the timer
				if !d.timer.Stop() {
					select {
					case <-d.timer.C:
					default:
					}
				}
				d.timer.Reset(dur)
			case <-d.timer.C:
				d.timerActive.Store(false)
				d.timer = nil
				d.onHandleTrackChange()
				return
			}
		}
	}()
}
