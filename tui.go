package main

import (
	"fmt"
	"math"
	"time"

	"github.com/nsf/termbox-go"
)

const (
	colDef    = termbox.ColorDefault
	colWhite  = termbox.ColorWhite
	colRed    = termbox.ColorRed
	colGreen  = termbox.ColorGreen
	colYellow = termbox.ColorYellow
	colBlue   = termbox.ColorBlue
	colCyan   = termbox.ColorCyan
)

type TUIState struct {
	selectedParam int
	comp          *SoftKneeCompressor
	exit          bool
}

var paramNames = []string{
	"Threshold (dB)",
	"Ratio (1:x)",
	"Knee (dB)",
	"Attack (ms)",
	"Release (ms)",
	"Makeup Gain (dB)",
	"Auto Makeup",
	"Bypass",
}

func runTUI(comp *SoftKneeCompressor) {
	err := termbox.Init()
	if err != nil {
		fmt.Printf("Failed to initialize TUI: %v\n", err)
		return
	}
	defer termbox.Close()

	termbox.SetInputMode(termbox.InputEsc)

	state := &TUIState{
		comp: comp,
	}

	eventQueue := make(chan termbox.Event)
	go func() {
		for {
			eventQueue <- termbox.PollEvent()
		}
	}()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	draw(state)

	for !state.exit {
		select {
		case ev := <-eventQueue:
			if ev.Type == termbox.EventKey {
				handleKey(ev, state)
			} else if ev.Type == termbox.EventResize {
				draw(state)
			}
		case <-ticker.C:
			draw(state)
		}
	}
}

func handleKey(ev termbox.Event, s *TUIState) {
	if ev.Key == termbox.KeyEsc || ev.Ch == 'q' {
		s.exit = true
		return
	}

	// Navigation
	if ev.Key == termbox.KeyArrowUp {
		s.selectedParam--
		if s.selectedParam < 0 {
			s.selectedParam = len(paramNames) - 1
		}
	} else if ev.Key == termbox.KeyArrowDown {
		s.selectedParam++
		if s.selectedParam >= len(paramNames) {
			s.selectedParam = 0
		}
	}

	// Adjustment
	// Lock once to read/modify multiple times or just rely on setters locking
	// Setters lock internally, so it's safe.
	
	// Coarse/Fine steps
	// We'll use simple steps for now
	switch s.selectedParam {
	case 0: // Threshold
		change := 0.0
		if ev.Key == termbox.KeyArrowRight { change = 0.5 }
		if ev.Key == termbox.KeyArrowLeft { change = -0.5 }
		if change != 0 {
			s.comp.mu.Lock()
			newVal := s.comp.thresholdDB + change
			if newVal > 0 { newVal = 0 }
			if newVal < -60 { newVal = -60 }
			s.comp.thresholdDB = newVal
			s.comp.updateParameters()
			s.comp.mu.Unlock()
		}
	case 1: // Ratio
		change := 0.0
		if ev.Key == termbox.KeyArrowRight { change = 0.5 }
		if ev.Key == termbox.KeyArrowLeft { change = -0.5 }
		if change != 0 {
			s.comp.mu.Lock()
			newVal := s.comp.ratio + change
			if newVal < 1.0 { newVal = 1.0 }
			if newVal > 20.0 { newVal = 20.0 }
			s.comp.ratio = newVal
			s.comp.updateParameters()
			s.comp.mu.Unlock()
		}
	case 2: // Knee
		change := 0.0
		if ev.Key == termbox.KeyArrowRight { change = 1.0 }
		if ev.Key == termbox.KeyArrowLeft { change = -1.0 }
		if change != 0 {
			s.comp.mu.Lock()
			newVal := s.comp.kneeDB + change
			if newVal < 0 { newVal = 0 }
			if newVal > 24 { newVal = 24 }
			s.comp.kneeDB = newVal
			s.comp.updateParameters()
			s.comp.mu.Unlock()
		}
	case 3: // Attack
		change := 0.0
		if ev.Key == termbox.KeyArrowRight { change = 1.0 }
		if ev.Key == termbox.KeyArrowLeft { change = -1.0 }
		if change != 0 {
			s.comp.mu.Lock()
			newVal := s.comp.attackMs + change
			if newVal < 0.1 { newVal = 0.1 }
			s.comp.attackMs = newVal
			s.comp.updateTimeConstants()
			s.comp.mu.Unlock()
		}
	case 4: // Release
		change := 0.0
		if ev.Key == termbox.KeyArrowRight { change = 10.0 }
		if ev.Key == termbox.KeyArrowLeft { change = -10.0 }
		if change != 0 {
			s.comp.mu.Lock()
			newVal := s.comp.releaseMs + change
			if newVal < 1.0 { newVal = 1.0 }
			s.comp.releaseMs = newVal
			s.comp.updateTimeConstants()
			s.comp.mu.Unlock()
		}
	case 5: // Makeup
		change := 0.0
		if ev.Key == termbox.KeyArrowRight { change = 0.5 }
		if ev.Key == termbox.KeyArrowLeft { change = -0.5 }
		if change != 0 {
			s.comp.mu.Lock()
			s.comp.autoMakeup = false
			newVal := s.comp.makeupGainDB + change
			if newVal < -24 { newVal = -24 }
			if newVal > 24 { newVal = 24 }
			s.comp.makeupGainDB = newVal
			s.comp.updateParameters()
			s.comp.mu.Unlock()
		}
	case 6: // Auto Makeup
		if ev.Key == termbox.KeyArrowRight || ev.Key == termbox.KeyArrowLeft || ev.Key == termbox.KeyEnter {
			s.comp.SetAutoMakeup(!s.comp.autoMakeup)
		}
	case 7: // Bypass
		if ev.Key == termbox.KeyArrowRight || ev.Key == termbox.KeyArrowLeft || ev.Key == termbox.KeyEnter {
			s.comp.SetBypass(!s.comp.bypass)
		}
	}
}

func draw(s *TUIState) {
	termbox.Clear(colDef, colDef)

	// Header
	printTB(0, 0, colCyan, colDef, "PipeWire Audio Compressor (pw-comp) - Interactive Mode")
	printTB(0, 1, colDef, colDef, "Use Arrows to navigate/adjust. 'q' or Esc to quit.")
	printTB(0, 2, colDef, colDef, "----------------------------------------------------")

	// Parameters
	s.comp.mu.Lock()
	// Read values locally
	vals := []string{
		fmt.Sprintf("%.1f", s.comp.thresholdDB),
		fmt.Sprintf("%.1f", s.comp.ratio),
		fmt.Sprintf("%.1f", s.comp.kneeDB),
		fmt.Sprintf("%.1f", s.comp.attackMs),
		fmt.Sprintf("%.1f", s.comp.releaseMs),
		fmt.Sprintf("%.1f", s.comp.makeupGainDB),
		fmt.Sprintf("%v", s.comp.autoMakeup),
		fmt.Sprintf("%v", s.comp.bypass),
	}
	s.comp.mu.Unlock()

	for i, name := range paramNames {
		col := colWhite
		bg := colDef
		prefix := "  "
		if i == s.selectedParam {
			col = colDef // Black usually if bg is white
			bg = colWhite // Highlight
			prefix = "> "
		}
		printTB(0, 4+i, col, bg, fmt.Sprintf("% -20s %s", prefix+name, vals[i]))
	}

	// Metering
	meters := s.comp.GetMeters()
	y := 14
	printTB(0, y, colYellow, colDef, "Meters:")
	
	// Convert linear to dB for display
	linToDB := func(l float64) float64 {
		if l <= 1e-6 { return -60.0 }
		return 20 * math.Log10(l)
	}
	
	inL := linToDB(meters.InputL)
	inR := linToDB(meters.InputR)
	outL := linToDB(meters.OutputL)
	outR := linToDB(meters.OutputR)
	grL := linToDB(meters.GainReductionL) // This is gain (e.g. -3 dB), closer to 0 is less reduction.
	grR := linToDB(meters.GainReductionR)

	drawMeter(2, y+2, "In L ", inL, colGreen)
	drawMeter(2, y+3, "In R ", inR, colGreen)
	
	// GR is usually shown as reduction amount (positive dB). 
	// Gain of 0.5 is -6dB. Display should show "6dB" reduction.
	// Gain is <= 1.0. So meters.GainReductionL is e.g. 0.5.
	// GR dB = -20*log10(gain). If gain is 1, GR is 0.
	grL_disp := -grL 
	grR_disp := -grR
	if grL_disp < 0 { grL_disp = 0 } // Should be positive
	if grR_disp < 0 { grR_disp = 0 }

	drawMeter(2, y+5, "GR L ", grL_disp, colRed) // Red for reduction
	drawMeter(2, y+6, "GR R ", grR_disp, colRed)

	drawMeter(2, y+8, "Out L", outL, colBlue)
	drawMeter(2, y+9, "Out R", outR, colBlue)

	termbox.Flush()
}

func drawMeter(x, y int, label string, db float64, color termbox.Attribute) {
	// Range -60 to 0 for levels. 0 to 30 for GR.
	// Let's normalize to bars.
	// Width 40 chars.
	// Level: -60 is empty, 0 is full.
	
	barWidth := 40
	filled := 0
	
	if color == colRed {
		// GR logic: 0 to 24 dB range
		// 0 dB = empty, 24 dB = full
		ratio := db / 24.0
		if ratio > 1 { ratio = 1 }
		filled = int(ratio * float64(barWidth))
	} else {
		// Level logic: -60 to 6 dB range
		// -60 = empty, 0 = 35 chars, +6 = full
		minDB := -60.0
		maxDB := 6.0
		if db < minDB { db = minDB }
		if db > maxDB { db = maxDB }
		ratio := (db - minDB) / (maxDB - minDB)
		filled = int(ratio * float64(barWidth))
	}
	
	printTB(x, y, colDef, colDef, fmt.Sprintf("%s [%-6.1f dB] ", label, db))
	
	// Draw bar
	startX := x + 15
	for i := 0; i < barWidth; i++ {
		ch := ' '
		bg := colDef
		if i < filled {
			ch = '█'
			// Gradient colors?
			// Termbox limited colors.
			// Just use the passed color
		} else {
			ch = '░'
		}
		termbox.SetCell(startX+i, y, ch, color, bg)
	}
}

func printTB(x, y int, fg, bg termbox.Attribute, msg string) {
	for _, c := range msg {
		termbox.SetCell(x, y, c, fg, bg)
		x++
	}
}
