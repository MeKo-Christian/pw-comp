package main

import (
	"fmt"
	"math"
	"strconv"
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
		//nolint:forbidigo // TUI initialization error requires direct output
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
			switch ev.Type {
			case termbox.EventKey:
				handleKey(ev, state)
			case termbox.EventResize:
				draw(state)
			}
		case <-ticker.C:
			draw(state)
		}
	}
}

//nolint:gocyclo,cyclop,funlen // UI event handler with multiple parameter cases
func handleKey(ev termbox.Event, s *TUIState) {
	if ev.Key == termbox.KeyEsc || ev.Ch == 'q' {
		s.exit = true
		return
	}

	// Navigation
	switch ev.Key {
	case termbox.KeyArrowUp:
		s.selectedParam--
		if s.selectedParam < 0 {
			s.selectedParam = len(paramNames) - 1
		}
	case termbox.KeyArrowDown:
		s.selectedParam++
		if s.selectedParam >= len(paramNames) {
			s.selectedParam = 0
		}
	}

	// Adjustment
	switch s.selectedParam {
	case 0: // Threshold
		change := 0.0
		if ev.Key == termbox.KeyArrowRight {
			change = 0.5
		}

		if ev.Key == termbox.KeyArrowLeft {
			change = -0.5
		}

		if change != 0 {
			s.comp.SetThreshold(s.comp.thresholdDB + change)
		}
	case 1: // Ratio
		change := 0.0
		if ev.Key == termbox.KeyArrowRight {
			change = 0.5
		}

		if ev.Key == termbox.KeyArrowLeft {
			change = -0.5
		}

		if change != 0 {
			s.comp.SetRatio(s.comp.ratio + change)
		}
	case 2: // Knee
		change := 0.0
		if ev.Key == termbox.KeyArrowRight {
			change = 1.0
		}

		if ev.Key == termbox.KeyArrowLeft {
			change = -1.0
		}

		if change != 0 {
			s.comp.SetKnee(s.comp.kneeDB + change)
		}
	case 3: // Attack
		change := 0.0
		if ev.Key == termbox.KeyArrowRight {
			change = 1.0
		}

		if ev.Key == termbox.KeyArrowLeft {
			change = -1.0
		}

		if change != 0 {
			s.comp.SetAttack(s.comp.attackMs + change)
		}
	case 4: // Release
		change := 0.0
		if ev.Key == termbox.KeyArrowRight {
			change = 10.0
		}

		if ev.Key == termbox.KeyArrowLeft {
			change = -10.0
		}

		if change != 0 {
			s.comp.SetRelease(s.comp.releaseMs + change)
		}
	case 5: // Makeup
		change := 0.0
		if ev.Key == termbox.KeyArrowRight {
			change = 0.5
		}

		if ev.Key == termbox.KeyArrowLeft {
			change = -0.5
		}

		if change != 0 {
			s.comp.SetMakeupGain(s.comp.makeupGainDB + change)
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

func draw(state *TUIState) {
	_ = termbox.Clear(colDef, colDef)

	meters := state.comp.GetMeters()

	// Header
	printTB(0, 0, colCyan, colDef, "PipeWire Audio Compressor (pw-comp) - Interactive Mode")
	printTB(0, 1, colWhite, colDef,
		fmt.Sprintf("Sample Rate: %.0f Hz | Processed Blocks: %d", meters.SampleRate, meters.Blocks))
	printTB(0, 2, colDef, colDef, "Use Arrows to navigate/adjust. 'q' or Esc to quit.")
	printTB(0, 3, colDef, colDef, "----------------------------------------------------")

	// Parameters
	state.comp.mu.Lock()
	vals := []string{
		fmt.Sprintf("%.1f", state.comp.thresholdDB),
		fmt.Sprintf("%.1f", state.comp.ratio),
		fmt.Sprintf("%.1f", state.comp.kneeDB),
		fmt.Sprintf("%.1f", state.comp.attackMs),
		fmt.Sprintf("%.1f", state.comp.releaseMs),
		fmt.Sprintf("%.1f", state.comp.makeupGainDB),
		strconv.FormatBool(state.comp.autoMakeup),
		strconv.FormatBool(state.comp.bypass),
	}
	state.comp.mu.Unlock()

	for i, name := range paramNames {
		col := colWhite
		bgColor := colDef
		prefix := "  "

		if i == state.selectedParam {
			col = colDef       // Black usually if bg is white
			bgColor = colWhite // Highlight
			prefix = "> "
		}

		printTB(0, 5+i, col, bgColor, fmt.Sprintf("% -20s %s", prefix+name, vals[i]))
	}

	// Metering
	meterY := 15
	printTB(0, meterY, colYellow, colDef, "Meters:")

	// Convert linear to dB for display
	linToDB := func(l float64) float64 {
		if l <= 1e-9 {
			return -96.0
		} // Lower noise floor

		return 20 * math.Log10(l)
	}

	inL := linToDB(meters.InputL)
	inR := linToDB(meters.InputR)
	outL := linToDB(meters.OutputL)
	outR := linToDB(meters.OutputR)
	grL := linToDB(meters.GainReductionL)
	grR := linToDB(meters.GainReductionR)

	drawMeter(meterY+2, "In L ", inL, colGreen)
	drawMeter(meterY+3, "In R ", inR, colGreen)

	grLeftDisp := -grL
	grRightDisp := -grR

	if grLeftDisp < 0 {
		grLeftDisp = 0
	}

	if grRightDisp < 0 {
		grRightDisp = 0
	}

	drawMeter(meterY+5, "GR L ", grLeftDisp, colRed)
	drawMeter(meterY+6, "GR R ", grRightDisp, colRed)

	drawMeter(meterY+8, "Out L", outL, colBlue)
	drawMeter(meterY+9, "Out R", outR, colBlue)

	termbox.Flush()
}

func drawMeter(yPos int, label string, db float64, color termbox.Attribute) {
	// Range -96 to +6 for levels, 0 to 30 for GR.
	const (
		barWidth = 60
		xPos     = 2
	)

	var filled int

	if color == colRed {
		// GR logic: 0 to 24 dB range
		// 0 dB = empty, 24 dB = full
		ratio := db / 24.0
		if ratio > 1 {
			ratio = 1
		}

		filled = int(ratio * float64(barWidth))
	} else {
		// Level logic: -96 to 6 dB range
		minDB := -96.0
		maxDB := 6.0

		if db < minDB {
			db = minDB
		}

		if db > maxDB {
			db = maxDB
		}

		ratio := (db - minDB) / (maxDB - minDB)
		filled = int(ratio * float64(barWidth))
	}

	printTB(xPos, yPos, colDef, colDef, fmt.Sprintf("%s [%-6.1f dB] ", label, db))

	// Draw bar
	startX := xPos + 15

	for i := range barWidth {
		var barChar rune
		bgCol := colDef

		if i < filled {
			barChar = '█'
		} else {
			barChar = '░'
		}

		termbox.SetCell(startX+i, yPos, barChar, color, bgCol)
	}
}

func printTB(x, y int, fg, bg termbox.Attribute, msg string) {
	for _, c := range msg {
		termbox.SetCell(x, y, c, fg, bg)
		x++
	}
}
