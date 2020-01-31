package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/go-msgpack/codec"
)

// snapshotHeader is the first entry in our snapshot
type snapshotHeader struct {
	// LastIndex is the last index that affects the data.
	// This is used when we do the restore for watchers.
	LastIndex uint64
}

type typeStats struct {
	Name       string
	Sum, Count int
}

type statSlice []typeStats

func (s statSlice) Len() int { return len(s) }

// Less sorts by size descending
func (s statSlice) Less(i, j int) bool { return s[i].Sum > s[j].Sum }
func (s statSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

var typeNames []string

func init() {
	// I have commented guesses as to what these additional types are, based on:
	// https://github.com/hashicorp/consul/blob/master/agent/consul/fsm/snapshot_oss.go
	typeNames = []string{
		"Register",
		"Deregister",
		"KVS",
		"Session",
		"ACL (Deprecated)",
		"Tombstone",
		"CoordinateBatchUpdate",
		"PreparedQuery",
		"Txn",
		"Autopilot",
		"Area",
		"ACLBootstrap",
		"Intention",
		"ConnectCA",
		"ConnectCAProviderState",
		"ConnectCAConfig",
		"Index",
		"ACLTokenSet",
		"ACLTokenDelete",
		"ACLPolicySet",
		"ACLPolicyDelete",
		"ConsulType1",  // ConfigEntrySet
		"ConsulType2",  // ConfigEntryDelete
		"ConsulType3",  // ACLRoleSet
		"ConsulType4",  // ACLRoleDelete
		"ConsulType5",  // ConfigEntrySet
		"ConsulType6",  // ConfigEntryDelete
		"ConsulType7",  // ACLBindingRuleSet
		"ConsulType8",  // ACLBindingRuleDelete
		"ConsulType9",  // ACLAuthMethodSet
		"ConsulType10", // ACLAuthMethodDelete
	}
}

type countingReader struct {
	r    io.Reader
	read int
}

func (r *countingReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	if err == nil {
		r.read += n
	}
	return n, err
}

func main() {

	// msgpackHandle is a shared handle for encoding/decoding msgpack payloads
	var msgpackHandle = &codec.MsgpackHandle{
		RawToString: true,
	}

	stats := make(map[int]typeStats)

	cr := &countingReader{r: os.Stdin}

	dec := codec.NewDecoder(cr, msgpackHandle)

	// Read in the header
	var header snapshotHeader
	if err := dec.Decode(&header); err != nil {
		panic(err)
	}

	// Populate the new state
	msgType := make([]byte, 1)
	offset := 0
	for {
		// Read the message type
		_, err := cr.Read(msgType)
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}

		// Decode
		s := stats[int(msgType[0])]
		if s.Name == "" {
			s.Name = typeNames[int(msgType[0])]
		}

		var val interface{}

		err = dec.Decode(&val)
		if err != nil {
			panic(err)
		}

		// See how big it was
		size := cr.read - offset

		s.Sum += size
		s.Count++
		offset += size

		stats[int(msgType[0])] = s
	}

	// Output stats in size-order
	ss := make(statSlice, 0, len(stats))

	for _, s := range stats {
		ss = append(ss, s)
	}

	// Sort the stat slice
	sort.Sort(ss)

	fmt.Printf("% 22s % 8s % 12s\n", "Record Type", "Count", "Total Size")
	fmt.Printf("%s %s %s\n", strings.Repeat("-", 22), strings.Repeat("-", 8), strings.Repeat("-", 12))
	for _, s := range ss {
		fmt.Printf("% 22s % 8d % 12s\n", s.Name, s.Count, ByteSize(uint64(s.Sum)))
	}
	fmt.Printf("%s %s %s\n", strings.Repeat("-", 22), strings.Repeat("-", 8), strings.Repeat("-", 12))
	fmt.Printf("%s % 8s % 12s\n", strings.Repeat(" ", 22), "TOTAL:", ByteSize(uint64(offset)))
}

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
	GIGABYTE
	TERABYTE
)

// ByteSize returns a human-readable byte string of the form 10M, 12.5K, and so forth.  The following units are available:
//	T: Terabyte
//	G: Gigabyte
//	M: Megabyte
//	K: Kilobyte
//	B: Byte
// The unit that results in the smallest number greater than or equal to 1 is always chosen.
// From https://github.com/cloudfoundry/bytefmt/blob/master/bytes.go
func ByteSize(bytes uint64) string {
	unit := ""
	value := float64(bytes)

	switch {
	case bytes >= TERABYTE:
		unit = "TB"
		value = value / TERABYTE
	case bytes >= GIGABYTE:
		unit = "GB"
		value = value / GIGABYTE
	case bytes >= MEGABYTE:
		unit = "MB"
		value = value / MEGABYTE
	case bytes >= KILOBYTE:
		unit = "KB"
		value = value / KILOBYTE
	case bytes >= BYTE:
		unit = "B"
	case bytes == 0:
		return "0"
	}

	result := strconv.FormatFloat(value, 'f', 1, 64)
	result = strings.TrimSuffix(result, ".0")
	return result + unit
}
