//go:build !unix

package storage

import "os"

func openNonblocking(root *os.Root, name string) (*os.File, error) { return root.Open(name) }

func openNonblockingLinks(root *os.Root, name string) (*os.File, error) { return root.Open(name) }
