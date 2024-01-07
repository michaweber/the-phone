package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
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

var (
	isPhonePickedUp      bool
	isResetButtonPressed bool
	isDialing            bool
	currentNumber        int
	dialedNumber         string
)

func main() {

	printBanner()

	/*
	 *
	 */
	fmt.Println("initializing number dial")
	numberDialLine, err := requestLine(rpi.J8p10, dialingHandler)
	if err != nil {
		fmt.Println("Unable to initialize number dial: %s\n", err)
		os.Exit(1)
	}
	defer numberDialLine.Close()

	/*
	 * Initializing Dialwheel
	 */
	fmt.Println("initializing dialing active")
	dialingActive, err := requestLine(rpi.J8p8, dialingActiveHandler)
	if err != nil {
		fmt.Printf("Unable to initialize dialing active: %s\n", err)
		os.Exit(1)
	}
	defer dialingActive.Close()

	/*
	 * Initializing Reset Button
	 */
	fmt.Println("initializing reset button")
	resetButtonLine, err := requestLine(rpi.J8p22, resetButtonHandler)
	if err != nil {
		fmt.Printf("Unable to initialize reset button: %s\n", err)
		os.Exit(1)
	}
	defer resetButtonLine.Close()

	/*
	 * Initializing Headset Hook
	 */
	fmt.Println("initializing headset hook..")
	headsetHookLine, err := requestLine(rpi.J8p16, headsetHookHandler)
	if err != nil {
		fmt.Printf("Unable to initialize headset hook: %s\n", err)
		os.Exit(1)
	}
	defer headsetHookLine.Close()

	// Read the mp3 file into memory
	fileBytes, err := os.ReadFile("./my-file.mp3")
	if err != nil {
		panic("reading my-file.mp3 failed: " + err.Error())
	}

	// Convert the pure bytes into a reader object that can be used with the mp3 decoder
	fileBytesReader := bytes.NewReader(fileBytes)

	// Decode file
	decodedMp3, err := mp3.NewDecoder(fileBytesReader)
	if err != nil {
		panic("mp3.NewDecoder failed: " + err.Error())
	}

	// Prepare an Oto context (this will use your default audio device) that will
	// play all our sounds. Its configuration can't be changed later.

	op := &oto.NewContextOptions{}

	// Usually 44100 or 48000. Other values might cause distortions in Oto
	op.SampleRate = 44100

	// Number of channels (aka locations) to play sounds from. Either 1 or 2.
	// 1 is mono sound, and 2 is stereo (most speakers are stereo).
	op.ChannelCount = 2

	// Format of the source. go-mp3's format is signed 16bit integers.
	op.Format = oto.FormatSignedInt16LE

	// Remember that you should **not** create more than one context
	otoCtx, readyChan, err := oto.NewContext(op)
	if err != nil {
		panic("oto.NewContext failed: " + err.Error())
	}
	// It might take a bit for the hardware audio devices to be ready, so we wait on the channel.
	<-readyChan

	// Create a new 'player' that will handle our sound. Paused by default.
	player := otoCtx.NewPlayer(decodedMp3)

	// Play starts playing the sound and returns without waiting for it (Play() is async).
	player.Play()

	// We can wait for the sound to finish playing using something like this
	for player.IsPlaying() {
		time.Sleep(time.Millisecond)
	}

	// Now that the sound finished playing, we can restart from the beginning (or go to any location in the sound) using seek
	// newPos, err := player.(io.Seeker).Seek(0, io.SeekStart)
	// if err != nil{
	//     panic("player.Seek failed: " + err.Error())
	// }
	// println("Player is now at position:", newPos)
	// player.Play()

	// If you don't want the player/sound anymore simply close
	err = player.Close()
	if err != nil {
		panic("player.Close failed: " + err.Error())
	}

	/*
	 * Main Loop to keep things running
	 */
	for {
		time.Sleep(time.Second / 2)
		printStatus()
	}

}

func requestLine(offset int, e gpiod.EventHandler) (*gpiod.Line, error) {
	debouncePeriod := 10 * time.Millisecond
	return gpiod.RequestLine("gpiochip0", offset,
		gpiod.WithPullUp,
		gpiod.WithBothEdges,
		gpiod.WithDebounce(debouncePeriod),
		gpiod.WithEventHandler(e))

}

func getValue(v bool) string {
	result := "N"
	if v {
		result = "Y"
	}
	return result
}

func printStatus() {
	fmt.Printf(
		"PickedUp: %v, Dialing: %v, ResetPressed: %v, Number: %v, Dialed: %v \n",
		getValue(isPhonePickedUp),
		getValue(isDialing),
		getValue(isResetButtonPressed),
		currentNumber,
		dialedNumber,
	)

}

func setCurrentNumber() {
	currentNumber += 1
	if currentNumber == 10 {
		currentNumber = 0
	}
}

func dialingHandler(evt gpiod.LineEvent) {
	switch evt.Type {
	case gpiod.LineEventFallingEdge:
	case gpiod.LineEventRisingEdge:
		setCurrentNumber()
	default:
	}

}

func dialingActiveHandler(evt gpiod.LineEvent) {
	switch evt.Type {
	case gpiod.LineEventFallingEdge:
		isDialing = true
		currentNumber = 0
	case gpiod.LineEventRisingEdge:
		isDialing = false
		dialedNumber += strconv.Itoa(currentNumber)
	default:
	}
}

func resetButtonHandler(evt gpiod.LineEvent) {
	switch evt.Type {
	case gpiod.LineEventFallingEdge:
		isResetButtonPressed = true
		dialedNumber = ""
	case gpiod.LineEventRisingEdge:
		isResetButtonPressed = false
	default:
	}
}

func startDialTone() {}
func stopDialTone()  {}

func headsetHookHandler(evt gpiod.LineEvent) {
	switch evt.Type {
	case gpiod.LineEventFallingEdge:
		isPhonePickedUp = true
		startDialTone()
	case gpiod.LineEventRisingEdge:
		isPhonePickedUp = false
	default:
	}
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

}

/*

To implement a countdown event handler in Go, you can use goroutines and channels. Here's an example:

```go
package main

import (
	"fmt"
	"time"
)

func countdown(count int, done chan bool) {
	for i := count; i > 0; i-- {
		fmt.Println(i)
		time.Sleep(time.Second)
	}

	done <- true
}

func main() {
	done := make(chan bool)

	go countdown(10, done)

	// Wait for the countdown to finish
	<-done

	fmt.Println("Countdown finished!")
}
```

In the above code, we define a `countdown` function that takes a starting count and a channel called `done`. It iterates from the starting count down to 1, printing the current count and sleeping for one second between each iteration.

Once the countdown is finished, it sends a value of `true` to the `done` channel to indicate that it's done.

In the `main` function, we create the `done` channel and start the countdown in a goroutine by calling `go countdown(10, done)`. This allows the countdown to run concurrently with the rest of the program.

Finally, we wait for the countdown to finish by reading from the `done` channel (`<-done`). Once we receive a value from the channel, we print "Countdown finished!".

You can modify the `countdown` function and the main logic according to your specific requirements.

*/
