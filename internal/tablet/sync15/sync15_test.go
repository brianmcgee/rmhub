package sync15

import (
	"hash/crc32"
	"testing"
)

// TestCRCHeader checks the header against the CRC32C check value of
// "123456789", 0xE3069283.
func TestCRCHeader(t *testing.T) {
	if got := crcHeader(crc32.Checksum([]byte("123456789"), castagnoli)); got != "crc32c=4waSgw==" {
		t.Fatal(got)
	}
	v, ok, err := headerCRC("md5=xyz, crc32c=4waSgw==")
	if err != nil || !ok || v != 0xE3069283 {
		t.Fatalf("parse: %x %v %v", v, ok, err)
	}
	if _, ok, err := headerCRC(""); ok || err != nil {
		t.Fatal("empty header")
	}
	if _, _, err := headerCRC("crc32c=!!"); err == nil {
		t.Fatal("bad value accepted")
	}
}
