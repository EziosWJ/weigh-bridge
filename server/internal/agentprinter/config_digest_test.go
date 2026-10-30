package agentprinter

import "testing"

// The running Agent's digest is compared with the candidate configuration after
// a save-and-apply. Display metadata is Server-assigned and never written back
// to agent.env, so it must not change the digest; device parameters must.
func TestConfigDigestCoversDeviceConfigurationOnly(t *testing.T) {
	base := Defaults()
	base.Queue = "Aisino-SK820-raw"
	base.SerialPort = "/dev/pts/18"

	metadata := base
	metadata.SiteCode, metadata.SiteName, metadata.ScaleName = "NEW", "新厂", "新厂地磅"
	if ConfigDigest(metadata) != ConfigDigest(base) {
		t.Fatal("display metadata must not change the running config digest")
	}
	serial := base
	serial.SerialPort = "/dev/pts/4"
	if ConfigDigest(serial) == ConfigDigest(base) {
		t.Fatal("serial port change must change the running config digest")
	}
	queue := base
	queue.Queue = "other-queue"
	if ConfigDigest(queue) == ConfigDigest(base) {
		t.Fatal("print queue change must change the running config digest")
	}
	scale := base
	scale.ScaleCode = "002"
	if ConfigDigest(scale) == ConfigDigest(base) {
		t.Fatal("scale identity must stay part of the running config digest")
	}
}
