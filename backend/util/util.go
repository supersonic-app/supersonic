package util

import (
	"fmt"
	"io"
	"net"
	"os"
)

func CopyFile(srcPath, dstPath string) error {
	fin, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer fin.Close()

	fout, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer fout.Close()

	_, err = io.Copy(fout, fin)
	return err
}

func CopyFileWithMode(srcPath, dstPath string, mode os.FileMode) error {
	fin, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer fin.Close()
	fout, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if err := fout.Chmod(mode); err != nil {
		_ = fout.Close()
		return err
	}
	if _, err := io.Copy(fout, fin); err != nil {
		_ = fout.Close()
		return err
	}
	return fout.Close()
}

func GetLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no suitable interface found")
}
