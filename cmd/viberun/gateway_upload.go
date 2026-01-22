// Copyright (c) 2025 AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shayne/viberun/internal/mux"
	"github.com/shayne/viberun/internal/muxrpc"
)

var errUploadTimeout = errors.New("upload timed out")

func uploadFileOverGateway(gateway *gatewayClient, localPath string, remotePath string) error {
	if gateway == nil {
		return os.ErrInvalid
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	meta := muxrpc.UploadMeta{
		Target: "host",
		Path:   remotePath,
		Size:   info.Size(),
		Sized:  true,
	}
	stream, err := gateway.openStream("upload", meta)
	if err != nil {
		return err
	}
	file, err := os.Open(localPath)
	if err != nil {
		_ = stream.Close()
		return err
	}
	defer file.Close()
	if _, err := io.Copy(stream, file); err != nil {
		_ = stream.Close()
		return err
	}
	err = waitForUpload(stream)
	_ = stream.Close()
	if err != nil {
		if errors.Is(err, errUploadTimeout) {
			ok, verifyErr := verifyHostUpload(gateway, remotePath, info.Size())
			if verifyErr != nil {
				return verifyErr
			}
			if ok {
				return nil
			}
		}
		return err
	}
	return nil
}

func uploadContainerFile(gateway *gatewayClient, container string, path string, data []byte) error {
	if gateway == nil {
		return os.ErrInvalid
	}
	meta := muxrpc.UploadMeta{
		Target:    "container",
		Container: container,
		Path:      path,
		Size:      int64(len(data)),
		Sized:     true,
	}
	stream, err := gateway.openStream("upload", meta)
	if err != nil {
		return err
	}
	if _, err := io.Copy(stream, bytes.NewReader(data)); err != nil {
		_ = stream.Close()
		return err
	}
	err = waitForUpload(stream)
	_ = stream.Close()
	if err != nil {
		if errors.Is(err, errUploadTimeout) {
			ok, verifyErr := verifyContainerUpload(gateway, container, path, int64(len(data)))
			if verifyErr != nil {
				return verifyErr
			}
			if ok {
				return nil
			}
		}
		return err
	}
	return nil
}

func waitForUpload(stream *mux.Stream) error {
	if stream == nil {
		return os.ErrInvalid
	}
	resultCh := make(chan error, 1)
	go func() {
		msg, err := stream.ReceiveMsg()
		if err != nil {
			if errors.Is(err, io.EOF) {
				resultCh <- nil
				return
			}
			resultCh <- err
			return
		}
		var result muxrpc.UploadResult
		if err := json.Unmarshal(msg, &result); err != nil {
			resultCh <- err
			return
		}
		if strings.TrimSpace(result.Error) != "" {
			resultCh <- errors.New(result.Error)
			return
		}
		resultCh <- nil
	}()
	select {
	case err := <-resultCh:
		return err
	case <-time.After(10 * time.Second):
		_ = stream.Close()
		return errUploadTimeout
	}
}

func verifyContainerUpload(gateway *gatewayClient, container string, path string, size int64) (bool, error) {
	if gateway == nil {
		return false, os.ErrInvalid
	}
	if size < 0 {
		return false, errors.New("invalid upload size")
	}
	cmd := []string{"docker", "exec", container, "sh", "-c", "wc -c < " + shellQuote(path)}
	output, err := gateway.exec(cmd, "", nil)
	if err != nil {
		return false, err
	}
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return false, errors.New("empty size output")
	}
	got, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return false, err
	}
	return got == size, nil
}

func verifyHostUpload(gateway *gatewayClient, path string, size int64) (bool, error) {
	if gateway == nil {
		return false, os.ErrInvalid
	}
	if size < 0 {
		return false, errors.New("invalid upload size")
	}
	cmd := []string{"sh", "-c", "wc -c < " + shellQuote(path)}
	output, err := gateway.exec(cmd, "", nil)
	if err != nil {
		return false, err
	}
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return false, errors.New("empty size output")
	}
	got, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return false, err
	}
	return got == size, nil
}
