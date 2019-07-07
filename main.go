package main

import (
	"encoding/json"
	"fmt"
	"github.com/antchfx/jsonquery"
	"github.com/atotto/clipboard"
	"github.com/dirkarnez/seismometer/assets"
	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"
	"github.com/sqweek/dialog"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type _Config struct {
	Value     string
	Retrieved string
}

var (
	configMap sync.Map
	c         chan bool
	traceFile *os.File
)

func main() {
	raw, err := ioutil.ReadFile("cathay.json")
	if err != nil {
		fmt.Println("Config file not found. No configuration is loaded.")
	}

	traceFile, err = os.OpenFile("trace.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Cannot RW trace file")
	}
	defer traceFile.Close()

	var configArr []Config
	if err := json.Unmarshal(raw, &configArr); err != nil {
		fmt.Println("Cannot parse config file. No configuration is loaded.")
	}

	for _, config := range configArr {
		configMap.Store(config.Name, _Config{
			Value: config.Value,
		})
	}

	c = make(chan bool, 1)
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetIcon(assets.Data)

	configMap.Range(func(k, v interface{}) bool {
		var name = k.(string)
		var config = v.(_Config)

		itemClickHandler := systray.AddMenuItem(name, fmt.Sprintf("Click to get latest data for %s", name))
		go waitForClick(itemClickHandler, name, config)
		return true
	})

	mQuitOrig := systray.AddMenuItem("Quit", "Quit the app")
	go func() {
		<-mQuitOrig.ClickedCh
		systray.Quit()
	}()

	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				updateAll()
			case <-c:
				ticker.Stop()
			}
		}
	}()
}

func onExit() {
	c <- true
}

func waitForClick(itemClickHandler *systray.MenuItem, name string, config _Config) {
	for {
		select {
		case <-itemClickHandler.ClickedCh:
			beeep.Notify("Retrieving...", "You will be notified soon", "")
			data, err := getData(config)
			if err != nil {
				beeep.Alert("Failure", err.Error(), "")
			} else {
				update(name, data)
				err = clipboard.WriteAll(data)
				if err != nil {
					beeep.Alert("Failure", "Cannot copy to your clipboard", "")
				} else {
					beeep.Notify("Success", fmt.Sprintf("%s has been retreived and copied to your clipboard", data), "")
				}
			}
		}
	}
}

func getData(config _Config) (string, error) {
	value := config.Value
	tokens := strings.Split(value, "->")
	if len(tokens) > 1 {
		doc, err := jsonquery.LoadURL(tokens[0])
		if err != nil {
			return "", fmt.Errorf("%s", "Please check your internet")
		}

		nodeNameNode := jsonquery.FindOne(doc, tokens[1])
		if nodeNameNode != nil {
			return nodeNameNode.InnerText(), nil
		} else {
			return "", fmt.Errorf("%s", "Cannot parse remote source using rules provided")
		}
	} else {
		return value, nil
	}
}

func updateAll() {
	configMap.Range(func(k, v interface{}) bool {
		var name = k.(string)
		var config = v.(_Config)

		data, err := getData(config)
		if err == nil {
			update(name, data)
		}

		return true
	})
}

func update(name, newData string) {
	if v, ok := configMap.Load(name); ok {
		var item = v.(_Config)
		oldData := item.Retrieved

		if oldData != newData {
			if len(oldData) > 0 {
				ok := dialog.Message("%s", fmt.Sprintf("Do you want to record the changes of %s's data?", name)).YesNo()
				if ok {
					// write log
					err := trace(fmt.Sprintf("[%s] changed from %s to %s", name, oldData, newData))
					if err != nil {
						beeep.Notify("Success", "Have been recorded in trace.txt", "")
					}
				}
			}
			
			item.Retrieved = newData
			configMap.Store(name, item)
		}
	}
}

func trace(content string) error {
	_, err := traceFile.Write([]byte(fmt.Sprintf("%s: %s \n", time.Now().Format("2006-01-02 15:04:05"), content)));
	return err
}