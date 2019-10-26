package httputils

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ansel1/merry"
)

func IncPortInAddr(addr string) (string, error) {
	hostAndPort := strings.Split(addr, ":")
	if len(hostAndPort) == 1 {
		return "", merry.New("missing port in '" + addr + "'")
	}
	host := hostAndPort[0]
	port, err := strconv.ParseInt(hostAndPort[1], 10, 64)
	if err != nil {
		return "", merry.Wrap(err)
	}
	return host + ":" + strconv.FormatInt(port+1, 10), nil
}

func RunBundleDevServer(devServerAddr, rootDir, hostArg, portArg string) error {
	hostAndPort := strings.Split(devServerAddr, ":")
	if len(hostAndPort) == 1 {
		return merry.New("missing port in '" + devServerAddr + "'")
	}
	cmd := exec.Command("npm", "run", "dev", "--", hostArg, hostAndPort[0], portArg, hostAndPort[1])
	cmd.Dir = filepath.Clean(rootDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return merry.Wrap(cmd.Start())
}

func RunBundleDevServerNear(srcAddress, rootDir, hostArg, portArg string) (string, error) {
	devServerAddr, err := IncPortInAddr(srcAddress)
	if err != nil {
		return "", merry.Wrap(err)
	}
	return devServerAddr, merry.Wrap(RunBundleDevServer(devServerAddr, rootDir, hostArg, portArg))
}

func LastBundleFName(bundlePath, prefix, suffix string) (string, error) {
	files, err := ioutil.ReadDir(bundlePath)
	if err != nil {
		return "", merry.Wrap(err)
	}
	var lastFName string
	var lastModTime time.Time
	for _, file := range files {
		if !file.IsDir() && lastModTime.Before(file.ModTime()) {
			name := file.Name()
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
				lastModTime = file.ModTime()
				lastFName = name
			}
		}
	}
	if lastFName == "" {
		return "", merry.Errorf("no bundles like %s*%s found in %s", prefix, suffix, bundlePath)
	}
	return lastFName, nil
}

func LastJSAndCSSFNames(bundlePath, jsPrefix, cssPrefix string) (string, string, error) {
	jsFPath, err := LastBundleFName(bundlePath, jsPrefix, ".js")
	if err != nil {
		return "", "", merry.Wrap(err)
	}
	cssFPath, err := LastBundleFName(bundlePath, cssPrefix, ".css")
	if err != nil {
		return "", "", merry.Wrap(err)
	}
	return jsFPath, cssFPath, nil
}
