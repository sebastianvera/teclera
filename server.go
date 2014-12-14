package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/go-martini/martini"
	"github.com/martini-contrib/cors"
	"github.com/martini-contrib/render"
	"github.com/tarm/goserial"
)

const (
	XbeeNodes              int    = 2
	serialPackageDelimeter string = ">"
)

type Response struct {
	Value int `json:"buttonPressed"`
	From  int `json:"address"`
}

func (r *Response) Reset() {
	r.Value = -1
	r.From = -1
}

func (r *Response) Answered() bool {
	return r.Value != -1 && r.From != -1
}

func (r *Response) Yes() int {
	return r.Value
}

func (r *Response) No() int {
	// If the response is 0, return a 1 and viceversa
	val := r.Value
	if val == 0 {
		val++
	} else {
		val = 0
	}

	return val
}

func (r *Response) TurnOnLedCommand() string {
	return fmt.Sprintf("AN %02x %02x", r.From, r.Value)
}

// Validate the response depending of the CurrentQuestionMode
func (r *Response) ValidResponse() bool {
	if CurrentQuestionMode == QuestionModes["two"] {
		return r.Value == 0 || r.Value == 1
	}

	if CurrentQuestionMode == QuestionModes["multiple"] {
		return r.Value >= 0 && r.Value <= 3
	}

	fmt.Printf("Error: Not a valid response\n CurrentQuestionMode: %v\nResponse: %+v\n", CurrentQuestionMode, r)
	return false
}

var (
	QuestionModes       = map[string]int{"two": 2, "multiple": 3}
	Responses           = make([]Response, XbeeNodes)
	CurrentQuestionMode = -1
	mutex               = &sync.Mutex{}
	serialConfig        = &serial.Config{Name: findArduino(), Baud: 9600}
)

var serialReader = 0

func main() {
	m := martini.Classic()

	m.Use(martini.Static("uploads", martini.StaticOptions{Prefix: "uploads"}))
	m.Use(render.Renderer())
	m.Use(cors.Allow(&cors.Options{
		AllowAllOrigins: true,
	}))

	m.Get("/", func(r render.Render) {
		r.HTML(200, "index", nil)
	})

	serialReader, err := serial.OpenPort(serialConfig)
	if err != nil {
		log.Fatal(err)
	}
	go CheckSerial(serialReader)
	m.Post("/upload", upload)
	m.Get("/uploads", listFiles)
	m.Post("/questions/start/:type", startQuestion)
	m.Post("/questions/stop", stopQuestion)
	m.Post("/test/:index/:val", test)
	m.Run()
}

func handleResponse(bytes []byte) {
	res := Response{}
	json.Unmarshal(bytes, &res)

	if Responses[res.From-1].Answered() {
		// fmt.Println("Response already answered")
	} else {
		if res.ValidResponse() {
			Responses[res.From-1] = res
			writeToSerial(res.TurnOnLedCommand())
		}
	}
}

func CheckSerial(serialBuffer io.ReadWriteCloser) {
	buff := bufio.NewReader(serialBuffer)
	for {
		bytes, _, err := buff.ReadLine()
		if err != nil {
			log.Fatal(err)
		}
		handleResponse(bytes)
	}
}

func writeToSerial(command string) {
	mutex.Lock()
	serialBuffer, err := serial.OpenPort(serialConfig)
	if err != nil {
		log.Fatal(err)
	}
	defer serialBuffer.Close()
	serialBuffer.Write([]byte(command + serialPackageDelimeter))
	mutex.Unlock()
	runtime.Gosched()
}

func findArduino() string {
	contents, _ := ioutil.ReadDir("/dev")

	for _, f := range contents {
		os := runtime.GOOS
		switch os {
		case "linux":
			if strings.Contains(f.Name(), "ACM") {
				return "/dev/" + f.Name()
			}
		case "darwin":
			if strings.Contains(f.Name(), "tty.usbmodem") {
				return "/dev/" + f.Name()
			}
		default:
			fmt.Errorf("Unknown Operating System: %s", os)
		}
	}

	return ""
}
func test(r render.Render, params martini.Params) {
	index, _ := strconv.Atoi(params["index"])
	val, _ := strconv.Atoi(params["val"])
	Responses[index].Value = val
	Responses[index].From = val
	fmt.Println(Responses)
	r.JSON(200, map[string]int{"index": index, "value": val, "from": Responses[index].From})
}

func stopQuestion(r render.Render) {
	jsonResponse := map[string]int{}
	switch CurrentQuestionMode {
	case QuestionModes["two"]:
		jsonResponse = map[string]int{"yes": 0, "no": 0}
		for _, response := range Responses {
			if response.Answered() {
				jsonResponse["yes"] += response.Yes()
				jsonResponse["no"] += response.No()
			}
		}
	case QuestionModes["multiple"]:
		jsonResponse = map[string]int{"a": 0, "b": 0, "c": 0, "d": 0}
		for _, response := range Responses {
			if response.Answered() {
				jsonResponse[string(byte('a'+response.Value))] += 1
			}
		}
	default:
		fmt.Errorf(
			"Bad Question Mode when calculating responses result %d",
			CurrentQuestionMode,
		)
	}
	writeToSerial(stopQuestionCommand())
	r.JSON(200, jsonResponse)
}

func stopQuestionCommand() string {
	return "Q0"
}

func startQuestionCommand() string {
	command := "Q1"

	switch CurrentQuestionMode {
	case QuestionModes["two"]:
		command += " TWO"
	case QuestionModes["multiple"]:
		command += " MUL"
	default:
		log.Fatal("Wrong question mode")
	}

	return command
}

func startQuestion(r render.Render, params martini.Params) {
	if mode, ok := QuestionModes[params["type"]]; ok {
		CurrentQuestionMode = mode
		tellArduinoToStartQuestion()
		resetQuestionsResponses()

		//TODO: Ask frontend guy if he needs some kind of feedback
		r.JSON(200, map[string]interface{}{"status": "started", "questionMode": mode})
	} else {
		r.JSON(422, map[string]interface{}{"status": "error", "msg": "Bad mode: " + params["type"]})
	}
}

func tellArduinoToStartQuestion() {
	writeToSerial(stopQuestionCommand())
	writeToSerial(startQuestionCommand())
}

func resetQuestionsResponses() {
	// fmt.Println("Resetting question responses")
	for i := range Responses {
		Responses[i].Reset()
	}
}

func upload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")

	defer file.Close()

	if err != nil {
		fmt.Fprintln(w, err)
		return
	}

	out, err := os.Create("./uploads/" + header.Filename)
	if err != nil {
		fmt.Fprintf(w, "Failed to open the file for writing")
		return
	}
	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		fmt.Fprintln(w, err)
	}

	// the header contains useful info, like the original file name
	fmt.Fprintf(w, "File %s uploaded successfully.", header.Filename)
}

func listFiles(r render.Render) {
	files, err := ioutil.ReadDir("./uploads")

	fileNames := []string{}

	for _, file := range files {
		if strings.Contains(file.Name(), ".pdf") {
			fileNames = append(fileNames, file.Name())
		}
	}

	if err != nil {
		r.JSON(500, map[string]interface{}{"error": err})
		return
	}

	r.JSON(200, map[string]interface{}{"files": fileNames})
}
