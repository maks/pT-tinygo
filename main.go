//go:build tinygo
// +build tinygo

package main

import (
	"image/color"
	"machine"
	"math/rand"
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
	INPUT_LT    = machine.Pin(12)
	INPUT_B     = machine.Pin(13)
	INPUT_A     = machine.Pin(14)
	INPUT_RT    = machine.Pin(15)
	INPUT_PLAY  = machine.Pin(16)
)

// Audio pins
const (
	AUDIO_PIO     = 0
	AUDIO_SM      = 0
	AUDIO_DMA     = 0
	AUDIO_DMA_IRQ = 0
	AUDIO_SDATA   = 17
	AUDIO_BCLK    = 18 // BCLK and LRCLK HAVE to be consecutive
	AUDIO_LRCLK   = 19
	NUM_SAMPLES   = 32 // Number of samples in our sine wave
	NUM_BLOCKS    = 4  // Number of blocks to buffer
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
	INPUT_A.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_B.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_LT.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
	INPUT_RT.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
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

	// Seed random number generator
	rand.Seed(time.Now().UnixNano())
	println("Random seed initialized")

	// Pre-clear the screen once before entering the loop
	display.FillScreen(colorBackground)
	display.Display()

	// Draw welcome message
	tinyfont.WriteLine(&display, &freemono.Regular12pt7b, 40, 100, "picoTracker", colorText)
	tinyfont.WriteLine(&display, &freemono.Regular9pt7b, 20, 150, "welcome from TinyGo!", colorText)
	tinyfont.WriteLine(&display, &freemono.Regular9pt7b, 20, 180, "Press START to play", colorText)
	display.Display()

	time.Sleep(200 * time.Millisecond)

	println("Starting main loop")

	// Main loop
	for {
		// Process button inputs first
		processInputs()

		// Long delay to prevent CPU hogging
		time.Sleep(16 * time.Millisecond)
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
		i2s := initSound()

		playTone(i2s)
	}
}

func initSound() *piolib.I2S {
	time.Sleep(time.Millisecond * 500)

	sm, _ := pio.PIO0.ClaimStateMachine()
	i2s, err := piolib.NewI2S(sm, AUDIO_SDATA, AUDIO_BCLK)
	if err != nil {
		panic(err.Error())
	}

	i2s.SetSampleFrequency(44100)
	return i2s
}

// Play a tone using PIO
// example from https://github.com/tinygo-org/pio/blob/master/rp2-pio/examples/i2s/main.go
func playTone(i2s *piolib.I2S) {
	// The amp on the picoTracker makes a VERY high output level so we need to reduce the volume alot
	// Volume control - reduce amplitude to 1% of original
	volume := 0.01 // 1% volume

	data := make([]uint32, NUM_SAMPLES*NUM_BLOCKS)
	for i := 0; i < NUM_SAMPLES*NUM_BLOCKS; i++ {
		// Scale down the amplitude by multiplying with volume factor
		scaledSample := int16(float64(sine[i%NUM_SAMPLES]) * volume)
		// Pack the scaled sample into both left and right channels
		data[i] = uint32(scaledSample) | uint32(scaledSample)<<16
	}

	// Play the sine wave for 2sec then off for 5sec
	for {
		for i := 0; i < 50; i++ {
			i2s.WriteStereo(data)
		}
	}
}
