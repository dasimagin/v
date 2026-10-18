package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os/exec"
	"time"
)

// codexModel is a model Codex offers to this account.
type codexModel struct {
	ID, Name string
	Efforts  []string
	Default  bool
}

// codexModels is filled on first use by asking `codex app-server`.
var codexModels []codexModel

// loadCodexModels asks Codex for its models once; it takes a second or two.
func loadCodexModels() ([]codexModel, error) {
	if codexModels != nil {
		return codexModels, nil
	}
	cmd := exec.Command("codex", "app-server")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	send := func(v any) { b, _ := json.Marshal(v); _, _ = in.Write(append(b, '\n')) }
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]any{"name": "v", "version": "0.1"}}})

	done := make(chan []codexModel, 1)
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			var m rpcMsg
			if json.Unmarshal(sc.Bytes(), &m) != nil {
				continue
			}
			switch string(m.ID) {
			case "1":
				send(map[string]any{"jsonrpc": "2.0", "method": "initialized"})
				send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "model/list", "params": map[string]any{}})
			case "2":
				var r struct {
					Data []struct {
						ID          string `json:"id"`
						DisplayName string `json:"displayName"`
						IsDefault   bool   `json:"isDefault"`
						Efforts     []struct {
							ReasoningEffort string `json:"reasoningEffort"`
						} `json:"supportedReasoningEfforts"`
					} `json:"data"`
				}
				_ = json.Unmarshal(m.Result, &r)
				var list []codexModel
				for _, d := range r.Data {
					cm := codexModel{ID: d.ID, Name: d.DisplayName, Default: d.IsDefault}
					for _, e := range d.Efforts {
						cm.Efforts = append(cm.Efforts, e.ReasoningEffort)
					}
					list = append(list, cm)
				}
				done <- list
				return
			}
		}
		done <- nil
	}()
	select {
	case list := <-done:
		if len(list) == 0 {
			return nil, errors.New("no models from codex; is `codex login` done?")
		}
		codexModels = list
		return list, nil
	case <-time.After(10 * time.Second):
		return nil, errors.New("codex did not answer in 10s")
	}
}

// codexModelName is the display name of a Codex model ID.
func codexModelName(id string) string {
	for _, m := range codexModels {
		if m.ID == id {
			return m.Name
		}
	}
	return id
}

// codexDefault is the model Codex uses when none is chosen.
func codexDefault() codexModel {
	for _, m := range codexModels {
		if m.Default {
			return m
		}
	}
	if len(codexModels) > 0 {
		return codexModels[0]
	}
	return codexModel{}
}
