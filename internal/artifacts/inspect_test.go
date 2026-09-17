package artifacts

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"strconv"
	"testing"

	"howett.net/plist"
)

func TestInspectIPAReadsExtensionAndProfile(t *testing.T) {
	ipa := zipArtifact(t, map[string][]byte{
		"Payload/Demo.app/Info.plist":                      plistXML(t, map[string]any{"CFBundleIdentifier": "com.example.demo", "CFBundleDisplayName": "Demo", "CFBundleShortVersionString": "1.2.3", "CFBundleVersion": "9", "MinimumOSVersion": "17.0", "CFBundleSupportedPlatforms": []any{"iPhoneOS"}}),
		"Payload/Demo.app/PlugIns/Widget.appex/Info.plist": plistXML(t, map[string]any{"CFBundleIdentifier": "com.example.demo.widget", "CFBundleDisplayName": "Widget"}),
		"Payload/Demo.app/embedded.mobileprovision":        []byte("ignore<?xml version=\"1.0\"?><plist version=\"1.0\"><dict><key>Name</key><string>Demo Profile</string><key>UUID</key><string>PROFILE-UUID</string><key>ExpirationDate</key><string>2030-01-01T00:00:00Z</string><key>ProvisionedDevices</key><array><string>device</string></array><key>Entitlements</key><dict><key>com.apple.developer.team-identifier</key><string>TEAM1</string><key>get-task-allow</key><false/></dict></dict></plist>tail"),
	})
	manifest, err := InspectIPA(ipa, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.BundleID != "com.example.demo" || manifest.Version != "1.2.3" || manifest.Status != "readable" {
		t.Fatalf("manifest = %+v", manifest)
	}
	if len(manifest.NestedBundles) != 1 || manifest.NestedBundles[0].BundleID != "com.example.demo.widget" {
		t.Fatalf("nested = %+v", manifest.NestedBundles)
	}
	if manifest.TeamID != "TEAM1" || manifest.Profile == nil || manifest.Profile.ProfileType != "ad-hoc" {
		t.Fatalf("profile = %+v team=%s", manifest.Profile, manifest.TeamID)
	}
	if manifest.Entitlements["com.apple.developer.team-identifier"] != "TEAM1" {
		t.Fatalf("entitlements = %#v", manifest.Entitlements)
	}
}

func TestInspectIPAUnsignedReturnsMetadata(t *testing.T) {
	ipa := zipArtifact(t, map[string][]byte{
		"Payload/Demo.app/Info.plist": plistXML(t, map[string]any{"CFBundleIdentifier": "com.example.demo"}),
	})
	manifest, err := InspectIPA(ipa, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "unsigned" || manifest.BundleID != "com.example.demo" {
		t.Fatalf("manifest = %+v", manifest)
	}
}

func TestInspectPKGReadsPackageInfo(t *testing.T) {
	info := []byte(`<pkg-info version="1.4.0" install-location="/Applications" identifier="com.example.pkg"><bundle id="com.example.demo" path="./Demo.app"/><bundle id="com.example.demo.widget" path="./Widget.appex"/></pkg-info>`)
	pkg := writeXar(t, map[string][]byte{"PackageInfo": info})
	manifest, err := InspectPKG(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ProductID != "com.example.pkg" || manifest.Version != "1.4.0" || manifest.InstallLocation != "/Applications" || manifest.Status != "readable" {
		t.Fatalf("manifest = %+v", manifest)
	}
	if len(manifest.BundleIDs) != 2 || manifest.BundleIDs[0] != "com.example.demo" {
		t.Fatalf("bundle IDs = %#v", manifest.BundleIDs)
	}
}

func zipArtifact(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, data := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func plistXML(t *testing.T, value map[string]any) []byte {
	t.Helper()
	data, err := plist.Marshal(value, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeXar(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var heap bytes.Buffer
	var filesXML bytes.Buffer
	id := 1
	for name, data := range files {
		offset := heap.Len()
		heap.Write(data)
		filesXML.WriteString(`<file id="` + strconv.Itoa(id) + `"><name>` + name + `</name><type>file</type><data><length>` + strconv.Itoa(len(data)) + `</length><offset>` + strconv.Itoa(offset) + `</offset><size>` + strconv.Itoa(len(data)) + `</size><encoding style="application/octet-stream"/></data></file>`)
		id++
	}
	toc := []byte(`<xar><toc>` + filesXML.String() + `</toc></xar>`)
	var compressed bytes.Buffer
	encoder := zlib.NewWriter(&compressed)
	if _, err := encoder.Write(toc); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 28)
	copy(header[:4], "xar!")
	binary.BigEndian.PutUint16(header[4:6], 28)
	binary.BigEndian.PutUint16(header[6:8], 1)
	binary.BigEndian.PutUint64(header[8:16], uint64(compressed.Len()))
	binary.BigEndian.PutUint64(header[16:24], uint64(len(toc)))
	binary.BigEndian.PutUint32(header[24:28], 0)
	return append(append(header, compressed.Bytes()...), heap.Bytes()...)
}
