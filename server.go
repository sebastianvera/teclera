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

	"github.com/go-martini/martini"
	"github.com/martini-contrib/cors"
	"github.com/martini-contrib/render"
	"github.com/tarm/goserial"
)

const MoteDevices int = 2

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

var (
	MoteModes           = map[string]int{"two": 0, "multiple": 1}
	Responses           = make([]Response, MoteDevices)
	CurrentQuestionMode = -1
)

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

	//TODO: Run serial daemon
	//TODO: Make sure that the serial daemon adds the questions

	go CheckSerial()
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

	// if is not set,
	if !Responses[res.From-1].Answered() {
		Responses[res.From-1] = res
		fmt.Println("New response")
		fmt.Printf("%+v\n", res)
	} else {
		fmt.Println("Response already answered")
	}
	fmt.Println("Responses:")
	for _, i := range Responses {
		fmt.Printf("%+v\n", i)
	}
}

func CheckSerial() {
	c := &serial.Config{Name: findArduino(), Baud: 9600}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	buff := bufio.NewReader(s)
	for {
		bytes, _, err := buff.ReadLine()
		if err != nil {
			log.Fatal(err)
		}
		handleResponse(bytes)
	}
}

func findArduino() string {
	contents, _ := ioutil.ReadDir("/dev")

	for _, f := range contents {
		os := runtime.GOOS
		switch os {
		case "linux":
			if strings.Contains(f.Name(), "ACM") ||
				strings.Contains(f.Name(), "ttyUSB3") {
				return "/dev/" + f.Name()
			}
		case "darwin":
			if strings.Contains(f.Name(), "tty.usbmodem") ||
				strings.Contains(f.Name(), "ttyUSB") {
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
	case MoteModes["two"]:
		jsonResponse = map[string]int{"yes": 0, "no": 0}
		for _, response := range Responses {
			if response.Answered() {
				jsonResponse["yes"] += response.Yes()
				jsonResponse["no"] += response.No()
			}
		}
	case MoteModes["multiple"]:
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
	r.JSON(200, jsonResponse)
}

func startQuestion(r render.Render, params martini.Params) {
	mode := MoteModes[params["type"]]

	tellMoteStartQuestion(mode)
	CurrentQuestionMode = mode
	resetQuestionsResponses()

	//TODO: Respond with a json
	r.JSON(200, map[string]string{"status": "started"})
}

func tellMoteStartQuestion(mode int) {
	fmt.Println("Mote start question mode", mode)
}

func resetQuestionsResponses() {
	fmt.Println("Resetting question responses")
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
