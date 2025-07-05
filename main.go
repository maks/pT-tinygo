//go:build tinygo
// +build tinygo

package main

import (
	"image/color"
	"machine"
	"strconv"
	"time"

	"tinygo.org/x/drivers/st7789"
	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/freemono"

	pio "github.com/tinygo-org/pio/rp2-pio"
	"github.com/tinygo-org/pio/rp2-pio/piolib"
)

// Display configuration
// Display SPI pins
const (
	DISPLAY_SPI_FREQ  = 20_000_000 // 20MHz
	DISPLAY_SCK_PIN   = machine.Pin(26)
	DISPLAY_SDO_PIN   = machine.Pin(27)
	DISPLAY_SDI_PIN   = machine.Pin(28) // Required for SPI config but not used by display
	DISPLAY_RESET_PIN = machine.Pin(22)
	DISPLAY_DC_PIN    = machine.Pin(21) // Data/Command pin
	DISPLAY_CS_PIN    = machine.Pin(20)
	DISPLAY_BACKLIGHT = machine.Pin(23)

	// Display dimensions
	DISPLAY_WIDTH    = 240
	DISPLAY_HEIGHT   = 320
	DISPLAY_ROTATION = 270 // Rotation in degrees
)

// SDIO pins
const (
	SDIO_CLK = 2
	SDIO_CMD = 3
	SDIO_D0  = 4
	SDIO_D1  = 5
	SDIO_D2  = 6
	SDIO_D3  = 7
)

// Input buttons configuration
const (
	INPUT_LEFT  = machine.Pin(8)
	INPUT_DOWN  = machine.Pin(9)
	INPUT_RIGHT = machine.Pin(10)
	INPUT_UP    = machine.Pin(11)
	INPUT_ALT   = machine.Pin(12)
	INPUT_EDIT  = machine.Pin(13)
	INPUT_ENTER = machine.Pin(14)
	INPUT_NAV   = machine.Pin(15)
	INPUT_PLAY  = machine.Pin(16)
)

// Audio configuration
const (
	AUDIO_SDATA = 17
	AUDIO_BCLK  = 18 // BCLK and LRCLK HAVE to be consecutive
	AUDIO_LRCLK = 19
	NUM_SAMPLES = 32    // Number of samples in one sine wave period
	NUM_BLOCKS  = 8     // Number of blocks to buffer
	SAMPLE_RATE = 44100 // Standard CD quality sample rate
)

// Battery voltage pin
const BATT_VOLTAGE_IN = 29

// UART configuration for debug output
const (
	DEBUG_UART_TX = machine.Pin(24)
	DEBUG_UART_RX = machine.Pin(25)
)

// colors
var (
	colorBackground = color.RGBA{0, 0, 0, 255}       // Black
	colorGrid       = color.RGBA{50, 50, 50, 255}    // Dark gray
	colorText       = color.RGBA{255, 255, 255, 255} // White
	colorRed        = color.RGBA{255, 0, 0, 255}     // Red
	colorBlue       = color.RGBA{0, 0, 255, 255}     // Blue
	colorGreen      = color.RGBA{0, 255, 0, 255}     // Green

	// Input debouncing
	lastButtonState  = make(map[machine.Pin]bool)
	lastDebounceTime = make(map[machine.Pin]int64)
	buttonState      = make(map[machine.Pin]bool)
)

// sine wave data
var sine []int16 = []int16{
	6392, 12539, 18204, 23169, 27244, 30272, 32137, 32767, 32137,
	30272, 27244, 23169, 18204, 12539, 6392, 0, -6393, -12540,
	-18205, -23170, -27245, -30273, -32138, -32767, -32138, -30273, -27245,
	-23170, -18205, -12540, -6393, -1,
}

// Update display with audio status
func updateAudioStatusDisplay() {
	display.FillRectangle(0, 190, 319, 20, colorBackground)
	statusText := "Audio: PLAYING"
	statusColor := colorGreen
	if !isAudioPlaying {
		statusText = "Audio: STOPPED"
		statusColor = colorRed
	}
	tinyfont.WriteLine(&display, &freemono.Regular9pt7b, 20, 200, statusText, statusColor)
	display.Display()
}

// Check if a button is pressed (with debouncing)
func isButtonPressed(pin machine.Pin) bool {
	reading := !pin.Get() // Inverted because of pull-up resistors

	// Initialize button state if not already done
	if _, exists := lastButtonState[pin]; !exists {
		lastButtonState[pin] = false
		lastDebounceTime[pin] = 0
		buttonState[pin] = false
	}

	now := time.Now().UnixNano()

	// If the button state changed, reset the debounce timer
	if reading != lastButtonState[pin] {
		lastDebounceTime[pin] = now
		lastButtonState[pin] = reading
	}

	// If the button state has been stable for the debounce delay
	if (now - lastDebounceTime[pin]) > 50_000_000 { // 50ms debounce
		// If the debounced state is different from the current state
		if reading != buttonState[pin] {
			buttonState[pin] = reading
			return buttonState[pin]
		}
	}

	return false
}

// Simple integer to string conversion
func itoa(val int) string {
	if val == 0 {
		return "0"
	}

	var result string
	for val > 0 {
		digit := val % 10
		result = string('0'+byte(digit)) + result
		val /= 10
	}

	return result
}

// Setup debug UART
// Setup debug UART
func setupPTDebugUART() {
	// Define custom pins
	txPin := machine.Pin(24) // Use GPIO 24 for TX
	rxPin := machine.Pin(25) // Use GPIO 25 for RX

	// Configure UART1
	uart1 := machine.UART1
	uart1.Configure(machine.UARTConfig{
		TX: txPin,
		RX: rxPin,
	})

	// Redirect standard output to UART0
	machine.Serial = uart1
	println("UART ready")
}

// Setup display
func setupDisplay() st7789.Device {
	// Configure SPI
	spi := machine.SPI1
	spiConfig := machine.SPIConfig{
		Frequency: DISPLAY_SPI_FREQ,
		SCK:       DISPLAY_SCK_PIN,
		SDO:       DISPLAY_SDO_PIN,
		SDI:       DISPLAY_SDI_PIN,
		Mode:      0,
	}
	err := spi.Configure(spiConfig)
	if err != nil {
		println("Failed to configure SPI:", err.Error())
		return st7789.Device{}
	}

	println("SPI configured successfully")

	// Configure display
	display := st7789.New(spi,
		DISPLAY_RESET_PIN,
		DISPLAY_DC_PIN,
		DISPLAY_CS_PIN,
		DISPLAY_BACKLIGHT,
	)

	println("Display created, now configuring...")

	// Initialize display
	display.Configure(st7789.Config{
		Width:        DISPLAY_WIDTH,
		Height:       DISPLAY_HEIGHT,
		Rotation:     st7789.Rotation(DISPLAY_ROTATION / 90),
		FrameRate:    st7789.FRAMERATE_60,
		RowOffset:    0,
		ColumnOffset: 0,
	})

	println("Display configured")

	// Give display time to initialize - longer delay
	time.Sleep(200 * time.Millisecond)

	display.InvertColors(true)
	println("Colors inverted")

	// Give display time to process inversion
	time.Sleep(50 * time.Millisecond)

	// Clear the display
	display.FillScreen(colorBackground)
	println("Screen cleared")

	// Wait for display to process the clear command
	time.Sleep(50 * time.Millisecond)

	println("Display ready")

	return display
}

// Configure input buttons
func setupButtons() {
	// Configure all buttons as inputs with pull-ups
	INPUT_LEFT.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_RIGHT.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_UP.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_DOWN.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_ENTER.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_EDIT.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_NAV.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_ALT.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_PLAY.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
}

var display st7789.Device

func main() {
	// Setup hardware
	setupPTDebugUART()
	println("PicoTracker TEST starting...")

	// Add a startup delay to ensure system is stable
	time.Sleep(500 * time.Millisecond)

	display = setupDisplay()
	println("Display setup complete")

	setupButtons()
	println("Buttons setup complete")

	// Pre-clear the screen once before entering the loop
	display.FillScreen(colorBackground)
	display.Display()

	// Draw welcome message
	tinyfont.WriteLine(&display, &freemono.Regular12pt7b, 40, 100, "picoTracker", colorText)
	tinyfont.WriteLine(&display, &freemono.Regular9pt7b, 20, 150, "welcome from TinyGo!", colorText)
	tinyfont.WriteLine(&display, &freemono.Regular9pt7b, 20, 180, "Press PLAY to start", colorText)
	display.Display()

	time.Sleep(200 * time.Millisecond)

	println("Starting main loop")

	// Initialize audio state tracking
	var lastAudioState = isAudioPlaying
	updateAudioStatusDisplay()

	initSound()

	// Main loop
	for {
		// Process button inputs first
		processInputs()

		// Update display if audio state changed
		if isAudioPlaying != lastAudioState {
			updateAudioStatusDisplay()
			lastAudioState = isAudioPlaying
		}

		// Handle any audio state updates (non-blocking)
		select {
		case state := <-audioStateChan:
			// Handle audio state changes if needed
			_ = state // Use the state if needed
		default:
			// No audio state changes
		}

		// Fixed frame rate delay
		time.Sleep(32 * time.Millisecond) // ~30 FPS
	}
}

var counter int = 0

// Process all button inputs based on current game state
func processInputs() { // Check for start button press
	if isButtonPressed(INPUT_PLAY) {
		println("Start button pressed!!")
		counter++
		// clear previous message that starts on 20,150
		display.FillRectangle(0, 170, 319, 20, colorBackground)
		// display message
		message := "START PRESSED: " + strconv.Itoa(counter)
		tinyfont.WriteLine(&display, &freemono.Regular9pt7b, 20, 180, message, colorBlue)
		display.Display()

		// Toggle audio playback
		toggleAudio()
	}
}

// Global buffer for audio data to avoid allocations
var (
	isAudioPlaying    = false
	audioPlaybackChan = make(chan bool, 1)
	audioStateChan    = make(chan bool, 1) // For non-blocking state updates
	audioI2S          *piolib.I2S
	audioBuffer       []uint32
)

// Initialize audio system
func initSound() *piolib.I2S {
	time.Sleep(100 * time.Millisecond) // Short delay for hardware to stabilize

	// Print debug info
	println("Initializing audio system...")
	println("Sample rate:", SAMPLE_RATE, "Hz")
	println("Sine wave period:", NUM_SAMPLES, "samples")
	println("Buffer size:", NUM_SAMPLES*8, "samples")

	// Initialize PIO state machine and I2S interface
	sm, err := pio.PIO0.ClaimStateMachine()
	if err != nil {
		println("Failed to claim state machine:", err.Error())
		return nil
	}

	i2s, err := piolib.NewI2S(sm, AUDIO_SDATA, AUDIO_BCLK)
	if err != nil {
		println("Failed to initialize I2S:", err.Error())
		return nil
	}

	// Set the sample rate with error checking
	err = i2s.SetSampleFrequency(SAMPLE_RATE)
	if err != nil {
		println("Warning: Failed to set sample rate:", err.Error())
	}
	println("I2S initialized at", SAMPLE_RATE, "Hz")

	// Sine wave data (32 samples for one period)
	var sine = [...]int16{
		6392, 12539, 18204, 23169, 27244, 30272, 32137, 32767, 32137,
		30272, 27244, 23169, 18204, 12539, 6392, 0, -6393, -12540,
		-18205, -23170, -27245, -30273, -32138, -32767, -32138, -30273, -27245,
		-23170, -18205, -12540, -6393, -1,
	}

	// Initialize the buffer only once
	if audioBuffer == nil {
		totalSamples := NUM_SAMPLES * 8 // 8 periods of the sine wave
		println("Allocating audio buffer with", totalSamples, "samples")
		audioBuffer = make([]uint32, totalSamples)

		// Fill the buffer with repeated periods of the sine wave
		for i := 0; i < totalSamples; i++ {
			// Scale down the amplitude (volume control)
			sample := int16((int32(sine[i%NUM_SAMPLES]) * 1) / 100) // 1% volume
			// Pack sample into both left and right channels
			audioBuffer[i] = uint32(uint16(sample)) | (uint32(uint16(sample)) << 16)
		}

		println("Audio buffer initialized with", len(audioBuffer), "samples")
	}

	// Store the I2S interface globally
	audioI2S = i2s

	// Start the audio playback goroutine
	go audioPlaybackLoop()

	return i2s
}

// Audio playback loop
func audioPlaybackLoop() {
	// Pre-calculate buffer size
	bufferSize := len(audioBuffer)
	if bufferSize == 0 {
		println("Error: Audio buffer not initialized")
		return
	}

	for {
		// Wait for playback to be enabled
		if !isAudioPlaying {
			if !<-audioPlaybackChan {
				continue
			}
		}

		// Play audio as long as isAudioPlaying is true
		for isAudioPlaying {
			// Write the audio buffer
			_, err := audioI2S.WriteStereo(audioBuffer)
			if err != nil {
				// Non-blocking error reporting
				select {
				case audioStateChan <- false: // Signal error state
				default:
				}
				time.Sleep(time.Millisecond)
				continue
			}
		}
	}
}

// Toggle audio playback
func toggleAudio() {
	isAudioPlaying = !isAudioPlaying
	// Send signal to audio goroutine
	audioPlaybackChan <- isAudioPlaying
}
