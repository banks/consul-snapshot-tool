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

type kvStats struct {
	Prefix     string
	Sum, Count int
}

type statSlice []typeStats
type kstatSlice []kvStats

func (s statSlice) Len() int  { return len(s) }
func (s kstatSlice) Len() int { return len(s) }

// Less sorts by size descending
func (s statSlice) Less(i, j int) bool  { return s[i].Sum > s[j].Sum }
func (s statSlice) Swap(i, j int)       { s[i], s[j] = s[j], s[i] }
func (s kstatSlice) Less(i, j int) bool { return s[i].Sum > s[j].Sum }
func (s kstatSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

var typeNames []string

func init() {
	// These mirror the const values from
	// https://github.com/hashicorp/consul/blob/master/agent/structs/structs.go#L37-L70
	// (line numbers may change but I want to link to master so it shows most recent
	// constants).
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
		"ConnectCALeafRequestType",
		"ConfigEntryRequestType",
		"ACLRoleSetRequestType",
		"ACLRoleDeleteRequestType",
		"ACLBindingRuleSetRequestType",
		"ACLBindingRuleDeleteRequestType",
		"ACLAuthMethodSetRequestType",
		"ACLAuthMethodDeleteRequestType",
		"ChunkingStateType",
		"FederationStateRequestType",
		"SystemMetadataRequestType",
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

	kstats := make(map[string]kvStats)

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
			if int(msgType[0]) < len(typeNames) {
				s.Name = typeNames[int(msgType[0])]
			} else {
				fmt.Printf("WARN: Unknown message type: %v\n", int(msgType[0]))
				fmt.Println("WARN: Probably needs updating from https://github.com/hashicorp/consul/blob/master/agent/structs/structs.go#L37-L70")
				fmt.Println()
			}
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

		if s.Name == "KVS" {
			switch val := val.(type) {
			case map[interface{}]interface{}:
				// depth controls how many levels deep we keep separate
				// kv stats for in the breakdown. this should probably
				// be a CLI option at some point.
				for k, v := range val {
					depth := 2
					if k == "Key" {
						split := strings.Split(v.(string), "/")
						if depth > len(split) {
							depth = len(split)
						}
						keys := split[0:depth]
						prefix := strings.Join(keys, "/")
						kvs := kstats[prefix]
						if kvs.Prefix == "" {
							kvs.Prefix = prefix
						}
						kvs.Sum += size
						kvs.Count++
						kstats[prefix] = kvs
					}
				}
			}
		}
		// fmt.Printf("%v\n", kstats)
		stats[int(msgType[0])] = s
	}

	// Output stats in size-order
	ss := make(statSlice, 0, len(stats))

	for _, s := range stats {
		ss = append(ss, s)
	}

	// Sort the stat slice
	sort.Sort(ss)

	fmt.Printf("%s\n", strings.Repeat("-", 52))
	fmt.Println("RECORD SUMMARY")
	fmt.Printf("%s\n", strings.Repeat("-", 52))
	fmt.Printf("% 30s % 8s % 12s\n", "Record Type", "Count", "Total Size")
	fmt.Printf("%s %s %s\n", strings.Repeat("-", 30), strings.Repeat("-", 8), strings.Repeat("-", 12))
	for _, s := range ss {
		fmt.Printf("% 30s % 8d % 12s\n", s.Name, s.Count, ByteSize(uint64(s.Sum)))
	}
	fmt.Printf("%s %s %s\n", strings.Repeat("-", 30), strings.Repeat("-", 8), strings.Repeat("-", 12))
	fmt.Printf("%s % 8s % 12s\n", strings.Repeat(" ", 30), "TOTAL:", ByteSize(uint64(offset)))

	if len(kstats) > 0 {
		fmt.Println()

		// Output key stats in size-order
		ks := make(kstatSlice, 0, len(kstats))

		for _, s := range kstats {
			ks = append(ks, s)
		}

		// Sort the key stat slice
		sort.Sort(ks)

		fmt.Printf("%s\n", strings.Repeat("-", 44))
		fmt.Println("KEY SIZE BREAKDOWN")
		fmt.Printf("%s\n", strings.Repeat("-", 44))
		fmt.Printf("% 22s % 8s % 12s\n", "Key Prefix", "Count", "Total Size")
		fmt.Printf("%s %s %s\n", strings.Repeat("-", 22), strings.Repeat("-", 8), strings.Repeat("-", 12))
		for _, s := range ks {
			fmt.Printf("% 22s % 8d % 12s\n", s.Prefix, s.Count, ByteSize(uint64(s.Sum)))
		}
		fmt.Printf("%s %s %s\n", strings.Repeat("-", 22), strings.Repeat("-", 8), strings.Repeat("-", 12))
		fmt.Printf("%s % 8s % 12s\n", strings.Repeat(" ", 22), "TOTAL:", ByteSize(uint64(offset)))
	}
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
