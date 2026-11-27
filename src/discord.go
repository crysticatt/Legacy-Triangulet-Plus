package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Discord Rich Presence over the local IPC pipe: "Opening Capsules / on Legacy Triangulet".
// The conversation is strictly request, then reply, so no background reader is needed
// (an always-blocking reader stalled writes on Windows pipes in an earlier version).

type discordConn struct {
	rw    io.ReadWriteCloser
	nonce int
}

func writeFrame(w io.Writer, op uint32, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	buf := make([]byte, 8+len(b))
	binary.LittleEndian.PutUint32(buf[0:], op)
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(b)))
	copy(buf[8:], b)
	_, err = w.Write(buf)
	return err
}

func readFrame(r io.Reader) (uint32, []byte, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[4:])
	if n > 1<<20 {
		return 0, nil, errors.New("oversized frame")
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return binary.LittleEndian.Uint32(hdr[0:]), body, nil
}

// roundTrip sends one frame and waits for the reply, giving up after the timeout.
func (d *discordConn) roundTrip(op uint32, v any, timeout time.Duration) ([]byte, error) {
	if err := writeFrame(d.rw, op, v); err != nil {
		return nil, err
	}
	type res struct {
		b   []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		_, b, err := readFrame(d.rw)
		ch <- res{b, err}
	}()
	select {
	case r := <-ch:
		return r.b, r.err
	case <-time.After(timeout):
		d.rw.Close() // unblocks the reader
		return nil, errors.New("discord did not answer")
	}
}

func discordHandshake(rw io.ReadWriteCloser, clientID string) (*discordConn, error) {
	d := &discordConn{rw: rw}
	b, err := d.roundTrip(0, map[string]any{"v": 1, "client_id": clientID}, 5*time.Second)
	if err != nil {
		return nil, err
	}
	var ev struct {
		Evt string `json:"evt"`
	}
	_ = json.Unmarshal(b, &ev)
	if ev.Evt != "READY" {
		return nil, fmt.Errorf("discord said %s", string(b))
	}
	return d, nil
}

// setActivity shows act, or clears the status when act is nil.
func (d *discordConn) setActivity(act map[string]any) error {
	d.nonce++
	args := map[string]any{"pid": os.Getpid()}
	if act != nil {
		args["activity"] = act
	}
	b, err := d.roundTrip(1, map[string]any{"cmd": "SET_ACTIVITY", "args": args, "nonce": fmt.Sprint(d.nonce)}, 5*time.Second)
	if err != nil {
		return err
	}
	var ev struct {
		Evt  string `json:"evt"`
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if json.Unmarshal(b, &ev) == nil && ev.Evt == "ERROR" {
		return errors.New(ev.Data.Message)
	}
	return nil
}

func (d *discordConn) close() { d.rw.Close() }

// presenceActivity builds the status text for the current state.
func presenceActivity(s Snapshot, since time.Time) map[string]any {
	state := "on Legacy Triangulet"
	if s.Opened > 0 {
		state = fmt.Sprintf("%s opened", fmtInt(s.Opened))
	}
	return map[string]any{
		"details":    "Opening Capsules",
		"state":      state,
		"timestamps": map[string]any{"start": since.Unix()},
		"assets":     map[string]any{"large_image": "logo", "large_text": "Triangulet Plus"},
	}
}

// RunDiscord keeps the presence up to date while the opener runs. It reconnects on its
// own if Discord is closed and opened again.
func RunDiscord(h *Hub, dial func(clientID string) (io.ReadWriteCloser, error), every time.Duration, stop <-chan struct{}) {
	var conn *discordConn
	var since time.Time
	shown := false
	for {
		cfg := h.cfg()
		s := h.Snapshot()
		if cfg.DiscordOn && s.Running {
			if since.IsZero() {
				since = time.Now()
			}
			if conn == nil {
				if rw, err := dial(cfg.DiscordID); err == nil {
					if c, err := discordHandshake(rw, cfg.DiscordID); err == nil {
						conn = c
					} else {
						rw.Close()
					}
				}
			}
			if conn != nil {
				if err := conn.setActivity(presenceActivity(s, since)); err != nil {
					conn.close()
					conn = nil
				} else {
					shown = true
				}
			}
		} else {
			since = time.Time{}
			if conn != nil && shown {
				if err := conn.setActivity(nil); err != nil {
					conn.close()
					conn = nil
				}
				shown = false
			}
		}
		select {
		case <-stop:
			if conn != nil {
				_ = conn.setActivity(nil)
				conn.close()
			}
			return
		case <-time.After(every):
		}
	}
}
