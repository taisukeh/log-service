package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/screwdriver-cd/log-service/sdstoreuploader"
)

// ----------------------------------------------------------------------------
// A bunch of mock and util stuff

type logMap map[string][]*logLine

var mockEmitterPath = "./data/emitterdata"

func mockEmitter() *os.File {
	e, err := os.Open(mockEmitterPath)
	if err != nil {
		panic(fmt.Errorf("Could not open fake emitter source: %v", err))
	}
	return e
}

type mockStepSaver struct {
	writeLog func(l *logLine) error
}

func (s mockStepSaver) Close() error {
	return nil
}

func (s mockStepSaver) WriteLog(l *logLine) error {
	if s.writeLog != nil {
		return s.writeLog(l)
	}
	return nil
}

func (s mockStepSaver) Write(p []byte) (int, error) {
	return len(p), nil
}

type mockSDStoreUploader struct {
	upload func(string, string) error
}

func (m *mockSDStoreUploader) Upload(path string, filePath string) error {
	if m.upload != nil {
		return m.upload(path, filePath)
	}

	return nil
}

func newTestApp() *mockApp {
	return &mockApp{}
}

func newRealApp() App {
	a := app{
		url:         "http://localhost:8080",
		emitterPath: "data/emitterdata",
		buildID:     "build123",
		token:       "faketoken",
	}

	return a
}

type mockApp struct {
	run         func()
	logReader   func() io.Reader
	uploader    func() sdstoreuploader.SDStoreUploader
	archiveLogs func(uploader sdstoreuploader.SDStoreUploader, src io.Reader) error
	stepSaver   func(step string) StepSaver
	buildID     string
}

func (a mockApp) Run() {
	if a.run != nil {
		a.run()
	}
}

func (a mockApp) LogReader() io.Reader {
	if a.logReader != nil {
		return a.logReader()
	}

	return mockEmitter()
}

func (a mockApp) Uploader() sdstoreuploader.SDStoreUploader {
	if a.uploader != nil {
		return a.uploader()
	}

	return &mockSDStoreUploader{}
}

func (a mockApp) BuildID() string {
	return a.buildID
}

func (a mockApp) StepSaver(step string) StepSaver {
	if a.stepSaver != nil {
		return a.stepSaver(step)
	}
	return &stepSaver{}
}

func parseLogFile(input *os.File) (logMap, error) {
	// Re-open the file so we don't need to seek to the beginning
	input, err := os.Open(input.Name())
	if err != nil {
		return nil, err
	}

	return parseLogData(input)
}

func parseLogData(input io.Reader) (logMap, error) {
	newLogs := logMap{}

	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		line := scanner.Text()
		newLog := &logLine{}
		if err := json.Unmarshal([]byte(line), newLog); err != nil {
			return nil, fmt.Errorf("unmarshaling log line %s: %v", line, err)
		}
		newLogs[newLog.Step] = append(newLogs[newLog.Step], newLog)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return newLogs, nil
}

// ----------------------------------------------------------------------------
// Actual tests below

func TestAppReader(t *testing.T) {
	want := bytes.NewBuffer(nil)
	f, _ := os.Open(mockEmitterPath)
	io.Copy(want, f)
	f.Close()

	a := newRealApp()
	got := bytes.NewBuffer(nil)
	io.Copy(got, a.LogReader())

	if got.String() != want.String() {
		t.Errorf("App.Reader() = %s, want %s", got, want)
	}
}

func TestArchiveLogsStepSaver(t *testing.T) {
	a := newTestApp()

	wantSteps := []string{
		"step0",
		"step1",
		"step2",
		"step3",
		"step4",
	}
	var gotSteps []string

	wantLogs, err := parseLogData(a.LogReader())
	if err != nil {
		t.Fatalf("Unexpected error fetching test data: %v", err)
	}

	// This is just here to be absolutely certain that the right data was loaded.
	// Delete if it gets annoying to add test data
	if len(wantLogs) != 5 {
		t.Errorf("Want %d logs, got %d", 5, len(wantLogs))
	}
	gotLogs := logMap{}

	a.stepSaver = func(step string) StepSaver {
		gotSteps = append(gotSteps, step)
		gotLogs[step] = []*logLine{}
		s := mockStepSaver{}
		s.writeLog = func(l *logLine) error {
			gotLogs[step] = append(gotLogs[step], l)
			return nil
		}
		return s
	}

	// This is the one line being tested...
	run(a)

	if len(gotLogs) != len(wantLogs) {
		t.Errorf("len(gotLogs) = %d, want %d. gotLogs = %v", len(gotLogs), len(wantLogs), gotLogs)
	}

	if len(gotSteps) != len(wantSteps) {
		t.Errorf("len(gotSteps) = %d, want %d. gotSteps = %v", len(gotSteps), len(wantSteps), gotSteps)
	}

	for i := range gotSteps {
		if gotSteps[i] != wantSteps[i] {
			t.Errorf("gotSteps[%d] = %s, want %s", i, gotSteps[i], wantSteps[i])
		}
	}

	for k, v := range gotLogs {
		if !reflect.DeepEqual(gotLogs[k], wantLogs[k]) {
			t.Errorf("\ngotLogs[%s] =\n  %s,\nwant\n  %s\n\n", k, v, wantLogs[k])
		}
	}
}
