package collector

import (
	"testing"
	"time"
)

const sampleNetstat = `Name       Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes  Coll
lo0        16384 <Link#1>                        946566     0 1316659316   946566     0 1316659316     0
en0        1500  <Link#14>   66:09:82:d0:42:b1 1591310908     0 2116339718063 601868919     0 640278788897     0
en0        1500  hologramdem fe80:e::86e:df7c: 1591310908     - 2116339718063 601868919     - 640278788897     -
`

func TestParseNetstat(t *testing.T) {
	stats, err := parseNetstat([]byte(sampleNetstat), "en0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.RxBytes != 2116339718063 {
		t.Errorf("RxBytes = %d, want 2116339718063", stats.RxBytes)
	}
	if stats.TxBytes != 640278788897 {
		t.Errorf("TxBytes = %d, want 640278788897", stats.TxBytes)
	}
}

func TestParseNetstatNotFound(t *testing.T) {
	_, err := parseNetstat([]byte(sampleNetstat), "en1")
	if err == nil {
		t.Fatal("expected error for missing interface")
	}
}

func TestDeltaRate(t *testing.T) {
	d := Delta{
		RxBytes:  1000000,
		TxBytes:  500000,
		Duration: 1 * time.Second,
	}
	if r := d.RxRate(); r != 1000000 {
		t.Errorf("RxRate = %f, want 1000000", r)
	}
	if r := d.TxRate(); r != 500000 {
		t.Errorf("TxRate = %f, want 500000", r)
	}
}

func TestDeltaRateZeroDuration(t *testing.T) {
	d := Delta{RxBytes: 100, TxBytes: 50, Duration: 0}
	if r := d.RxRate(); r != 0 {
		t.Errorf("RxRate = %f, want 0", r)
	}
}
