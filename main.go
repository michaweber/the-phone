package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/michaweber/thephone/config"
	"github.com/warthog618/gpiod"
	"github.com/warthog618/gpiod/device/rpi"
)

/*
 * Pinout for Raspberry Pi Zero
 *
 * J8:
 *    3V3  (1) (2)  5V
 *  GPIO2  (3) (4)  5V
 *  GPIO3  (5) (6)  GND    --- Dialwheel
 *  GPIO4  (7) (8)  GPIO14 --- Dialing Active
 *    GND  (9) (10) GPIO15 --- Numbers
 * GPIO17 (11) (12) GPIO18
 * GPIO27 (13) (14) GND    --- Headset
 * GPIO22 (15) (16) GPIO23 --- Headset
 *    3V3 (17) (18) GPIO24
 * GPIO10 (19) (20) GND    --- Reset
 *  GPIO9 (21) (22) GPIO25 --- Reset
 * GPIO11 (23) (24) GPIO8
 *    GND (25) (26) GPIO7
 *  GPIO0 (27) (28) GPIO1
 *  GPIO5 (29) (30) GND
 *  GPIO6 (31) (32) GPIO12
 * GPIO13 (33) (34) GND
 * GPIO19 (35) (36) GPIO16
 * GPIO26 (37) (38) GPIO20
 *    GND (39) (40) GPIO21
 */

const (
	callingTimer = 2
	path         = "/home/op/sounds/"
)

var (
	// indicates if handle is picked up or now
	isPhonePickedUp bool = false
	// the reset button to the left of the dial wheel
	isResetButtonPressed bool = false
	// true as long as the dial wheel is turning
	isDialing bool = false
	// the current digit which is dialing
	currentDigit int = 0
	// combination of all dialed digits
	dialedNumber string = ""
	// a timer to start the "phone call" 2 seconds after the last dialed digit
	dialTimer *time.Timer
	// flag to indicate if a "call" is currently active
	isCalling bool = false
	// little debug flag
	debug bool = false
)

/*
 * The main function...
 */
func main() {
	// print the banner, just because we can
	printBanner()

	// initialize number dial
	numberDialLine := initializeNumberDial()
	defer numberDialLine.Close()

	// initialize dial wheel
	dialingActive := initializeDialingActive()
	defer dialingActive.Close()

	// initialize reset button
	resetButtonLine := initializeResetButton()
	defer resetButtonLine.Close()

	// initialize headset hook
	headsetHookLine := initializeHeadsetHook()
	defer headsetHookLine.Close()

	// main loop to keep things running
	for {
		time.Sleep(time.Millisecond * 100)
		if debug {
			time.Sleep(time.Second * 1)
			printStatus()
		}
	}

}

/*
 * callNumberHandler is the main handler for the actual "call"
 * It receives the dialed number after the dialing timer has finished
 * and decides which action to take
 */
func callNumberHandler(number string) {
	printInfo(fmt.Sprintf("dialing number: %v", number))
	stopDialTone()
	isCalling = true
	switch number {
	case "1":
		play("connect.mp3")
	case "2":
		play("dialup.mp3")
	case "001":
		toggleDebugFlag()
	case "00424352968":
		play("hint.mp3")
	default:
		play("number-not-working.mp3")
	}
	reset()
}

/*
 * requestLine sets up the provided gpio pin to receive the signals
 */
func requestLine(offset int, e gpiod.EventHandler) (*gpiod.Line, error) {
	debouncePeriod := 10 * time.Millisecond
	return gpiod.RequestLine("gpiochip0", offset,
		gpiod.WithPullUp,
		gpiod.WithBothEdges,
		gpiod.WithDebounce(debouncePeriod),
		gpiod.WithEventHandler(e))
}

/*
 * initializeHeadsetHook initializes the gpio pins connected to the headset
 */
func initializeHeadsetHook() *gpiod.Line {
	printInfo("initializing headset hook..")
	headsetHookLine, err := requestLine(rpi.J8p16, headsetHookHandler)
	if err != nil {
		printError("Unable to initialize headset hook", err)
		os.Exit(1)
	}
	return headsetHookLine
}

/*
 * initializeResetButton initializes the gpiod pins connected to the button
 */
func initializeResetButton() *gpiod.Line {
	printInfo("initializing reset button")
	resetButtonLine, err := requestLine(rpi.J8p22, resetButtonHandler)
	if err != nil {
		printError("Unable to initialize reset button", err)
		os.Exit(1)
	}
	return resetButtonLine
}

/*
 * initializingDialingActive initializes the gpiod pins which indicate that
 * the dial wheel is active
 */
func initializeDialingActive() *gpiod.Line {
	printInfo("initializing dialing active")
	dialingActive, err := requestLine(rpi.J8p8, dialingActiveHandler)
	if err != nil {
		printError("Unable to initialize dialing active", err)
		os.Exit(1)
	}
	return dialingActive
}

func initializeNumberDial() *gpiod.Line {
	printInfo("initializing number dial")
	numberDialLine, err := requestLine(rpi.J8p10, dialingHandler)
	if err != nil {
		printError("Unable to initialize number dial", err)
		os.Exit(1)
	}
	return numberDialLine
}

/*
 * toggles the debug flag
 */
func toggleDebugFlag() {
	if !debug {
		debug = true
	} else {
		debug = false
	}
}

/*
 * printStatus prints the statusline if debug is true
 */
func printStatus() {
	printInfo(fmt.Sprintf(
		"PickedUp: %v, Dialing: %v, ResetPressed: %v, Digit: %v, Dialed: %v",
		getValue(isPhonePickedUp),
		getValue(isDialing),
		getValue(isResetButtonPressed),
		currentDigit,
		dialedNumber,
	))
}

/*
 * getValue is a small helper to translate bool into "N" and "Y" for the
 * print status line
 */
func getValue(v bool) string {
	result := "N"
	if v {
		result = "Y"
	}
	return result
}

/*
 * small helper to print some logging messages
 */
func printInfo(msg string) {
	fmt.Printf("INFO: %v \n", msg)
}

/*
 * small helper to print some logging error message
 */
func printError(msg string, err error) {
	fmt.Printf("ERR: %v: %s \n", msg, err)
}

/*
 * startDialingTimer provides a timer to wait the number of seconds dedined in
 * callingTimer after the last digit was dialed.
 * If the timer is finised, it invoces the callNumberHandler with the currently
 * dialed number.
 */
func startDialingTimer(number string) {
	stopDialingTimer()
	printInfo("starting Timer...")
	dialTimer = time.NewTimer(callingTimer * time.Second)
	go func() {
		<-dialTimer.C
		callNumberHandler(number)
	}()
}

/*
 * stopDialingTimer stops the timer is its running
 */
func stopDialingTimer() {
	if dialTimer != nil {
		printInfo("stoping timer")
		dialTimer.Stop()
	}

}

/*
 * play calls mpg123 with the provided sound file
 */
func play(file string) {
	cmd := exec.Command("mpg123", path+file)
	if err := cmd.Start(); err != nil {
		printError("Unable to play file", err)
	}
	cmd.Wait()
}

/*
 * setCurrentDigit counts up the current Digit every time it's called,
 * if the counter reaches 10, then the 0 was dialed
 */
func setCurrentDigit() {
	currentDigit += 1
	if currentDigit == 10 {
		currentDigit = 0
	}
}

/*
 * resetCurrentDigit resets the currently dialed digit to 0
 */
func resetCurrentDigit() {
	currentDigit = 0
}

/*
 * resetNumber resets the dialed number back to an empty string
 */
func resetNumber() {
	dialedNumber = ""
}

/*
 * reset sets all phone constants back ot their intial value
 */
func reset() {
	stopDialTone()
	isCalling = false

	stopDialingTimer()
	resetNumber()
	resetCurrentDigit()

	if isPhonePickedUp {
		startDialTone()
	}
}

/*
 * starts the dial tone
 */
func startDialTone() {
	printInfo("starting dial tone...")
	cmd := exec.Command(path + "freizeichen.sh")
	if err := cmd.Start(); err != nil {
		printError("Unable to play", err)
	}
}

/*
 * stopDialTone stops the dial tone sound, currently this means killing all
 * running mpg123 processes
 */
func stopDialTone() {
	cmd := "ps -aux|grep mpg123|grep -v grep"
	_, err := exec.Command("bash", "-c", cmd).Output()
	if err == nil {
		printInfo("stoping dial tone")
		exec.Command("killall", "mpg123").Output()
	}
}

/*
 * resetButtonHandler receives the signal if the reset button is pressed and
 * sets the isResetButtonPressed flag and resets the phone if pressed
 */
func resetButtonHandler(evt gpiod.LineEvent) {
	switch evt.Type {
	case gpiod.LineEventFallingEdge:
		isResetButtonPressed = true
		if isPhonePickedUp {
			reset()
		}
	case gpiod.LineEventRisingEdge:
		isResetButtonPressed = false
	default:
	}
}

/*
 * dialingHandler receives a signal every time a number is passed
 * by the dial wheel and calls setCurrentDigit to count the signals
 */
func dialingHandler(evt gpiod.LineEvent) {
	switch evt.Type {
	case gpiod.LineEventFallingEdge:
	case gpiod.LineEventRisingEdge:
		setCurrentDigit()
	default:
	}

}

/*
 * dialingActiveHandler receives a signal as long as the dial wheel is active
 */
func dialingActiveHandler(evt gpiod.LineEvent) {
	switch evt.Type {
	case gpiod.LineEventFallingEdge:
		isDialing = true
		stopDialingTimer()
		resetCurrentDigit()

	case gpiod.LineEventRisingEdge:
		isDialing = false
		if isPhonePickedUp && !isCalling {
			dialedNumber += strconv.Itoa(currentDigit)
			startDialingTimer(dialedNumber)
		}
	default:
	}
}

/*
 * headsetHookHandler receives a signal if the headset is picked up
 */
func headsetHookHandler(evt gpiod.LineEvent) {
	switch evt.Type {
	case gpiod.LineEventFallingEdge:
		isPhonePickedUp = true
	case gpiod.LineEventRisingEdge:
		isPhonePickedUp = false
	default:
	}
	reset()
}

func printBanner() {

	fmt.Println("  █████    █████                            █████                                  ")
	fmt.Println(" ░░███    ░░███                            ░░███                                   ")
	fmt.Println(" ███████   ░███████    ██████     ████████  ░███████    ██████  ████████    ██████ ")
	fmt.Println("░░░███░    ░███░░███  ███░░███   ░░███░░███ ░███░░███  ███░░███░░███░░███  ███░░███")
	fmt.Println("  ░███     ░███ ░███ ░███████     ░███ ░███ ░███ ░███ ░███ ░███ ░███ ░███ ░███████ ")
	fmt.Println("  ░███ ███ ░███ ░███ ░███░░░      ░███ ░███ ░███ ░███ ░███ ░███ ░███ ░███ ░███░░░  ")
	fmt.Println("  ░░█████  ████ █████░░██████     ░███████  ████ █████░░██████  ████ █████░░██████ ")
	fmt.Println("   ░░░░░  ░░░░ ░░░░░  ░░░░░░      ░███░░░  ░░░░ ░░░░░  ░░░░░░  ░░░░ ░░░░░  ░░░░░░  ")
	fmt.Println("                                  ░███                                             ")
	fmt.Println("                                  █████                                            ")
	fmt.Println("                                 ░░░░░                                             ")
	fmt.Printf(" \x1b[1;37mVersion:\x1b[0m %s_%s\n", config.Env, config.Version)
	fmt.Printf(" \x1b[1;37mBuildtime:\x1b[0m %s\n", config.Build)
}
