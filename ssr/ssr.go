package ssr

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"net/http"

	"github.com/ansel1/merry"
)

var ssrClient = &http.Client{}

func Prerender(ssrDaemonAddress string, params map[string]interface{}) (template.HTML, error) {
	bufReq := &bytes.Buffer{}
	err := json.NewEncoder(bufReq).Encode(params)
	if err != nil {
		return "", merry.Wrap(err)
	}

	resp, err := ssrClient.Post(ssrDaemonAddress+"/render", "application/json", bufReq)
	if err != nil {
		return "", merry.Prepend(err, "SSR: failed to send request")
	}
	if resp.StatusCode != 200 {
		return "", merry.New("SSR: failed with " + resp.Status)
	}
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", merry.Prepend(err, "SSR: failed to read response")
	}

	return template.HTML(buf), nil
}
