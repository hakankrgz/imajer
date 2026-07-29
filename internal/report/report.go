package report

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hakankrgz/imajer/internal/config"
	"github.com/hakankrgz/imajer/internal/evidence"
	"github.com/hakankrgz/imajer/internal/fsutil"
	"github.com/hakankrgz/imajer/internal/probe"
)

type CaseReport struct {
	Version      int                                 `json:"version"`
	GeneratedAt  time.Time                           `json:"generated_at"`
	Case         config.Case                         `json:"case"`
	Profile      string                              `json:"profile"`
	Target       probe.Info                          `json:"target"`
	LocalStorage fsutil.Details                      `json:"local_storage"`
	Artifacts    []evidence.State                    `json:"artifacts"`
	Sessions     map[string][]evidence.SessionRecord `json:"sessions,omitempty"`
	Tools        []ToolEvidence                      `json:"tools,omitempty"`
	Footprint    []string                            `json:"footprint"`
	Warnings     []string                            `json:"warnings"`
}

type ToolEvidence struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	OS         string `json:"os,omitempty"`
	Arch       string `json:"arch,omitempty"`
	Kernel     string `json:"kernel,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	License    string `json:"license,omitempty"`
	RemotePath string `json:"remote_path,omitempty"`
	Trust      string `json:"trust"`
}

type EvidenceIndex struct {
	Version      int                 `json:"version"`
	CreatedAt    time.Time           `json:"created_at"`
	CaseID       string              `json:"case_id"`
	EvidenceID   string              `json:"evidence_id"`
	SigningKeyID string              `json:"signing_key_id"`
	Files        []evidence.FileHash `json:"files"`
}

func WriteCaseReport(caseDir string, data CaseReport) error {
	data.Version = 1
	data.GeneratedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err := evidence.AtomicWrite(filepath.Join(caseDir, "case-report.json"), append(raw, '\n'), 0o600); err != nil {
		return err
	}
	lines := reportLines(data)
	return writePDF(filepath.Join(caseDir, "case-report.pdf"), lines)
}

func FinalizeIndex(caseDir, caseID, evidenceID, privateKeyPath string) (*EvidenceIndex, error) {
	if privateKeyPath == "" {
		return nil, errors.New("external Ed25519 signing key is required")
	}
	key, pub, err := loadSigningKey(privateKeyPath)
	if err != nil {
		return nil, err
	}
	files, err := hashTree(caseDir)
	if err != nil {
		return nil, err
	}
	pubHash := sha256.Sum256(pub)
	idx := &EvidenceIndex{
		Version: 1, CreatedAt: time.Now().UTC(), CaseID: caseID, EvidenceID: evidenceID,
		SigningKeyID: "sha256:" + hex.EncodeToString(pubHash[:]), Files: files,
	}
	raw, err := json.Marshal(idx)
	if err != nil {
		return nil, err
	}
	// The signed representation is also the exact representation stored on
	// disk. EvidenceIndex intentionally contains no maps, floats or interface
	// values, so encoding/json yields one deterministic, compact canonical
	// form for this schema.
	if err := evidence.AtomicWrite(filepath.Join(caseDir, "evidence-index.json"), raw, 0o600); err != nil {
		return nil, err
	}
	sig := ed25519.Sign(key, raw)
	if err := evidence.AtomicWrite(filepath.Join(caseDir, "evidence-index.sig"), []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o600); err != nil {
		return nil, err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(ed25519.PublicKey(pub))
	if err != nil {
		return nil, err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	if err := evidence.AtomicWrite(filepath.Join(caseDir, "examiner-public-key.pem"), pubPEM, 0o644); err != nil {
		return nil, err
	}
	return idx, nil
}

func VerifyIndex(caseDir string, trustedPublicKeyPath ...string) error {
	indexRaw, err := os.ReadFile(filepath.Join(caseDir, "evidence-index.json"))
	if err != nil {
		return err
	}
	var idx EvidenceIndex
	if err := json.Unmarshal(indexRaw, &idx); err != nil {
		return err
	}
	canonical, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	if !bytes.Equal(indexRaw, canonical) {
		return errors.New("evidence index is not in the required canonical JSON form")
	}
	sigRaw, err := os.ReadFile(filepath.Join(caseDir, "evidence-index.sig"))
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigRaw)))
	if err != nil {
		return err
	}
	pubPath := filepath.Join(caseDir, "examiner-public-key.pem")
	if len(trustedPublicKeyPath) > 0 && trustedPublicKeyPath[0] != "" {
		pubPath = trustedPublicKeyPath[0]
	}
	pubRaw, err := os.ReadFile(pubPath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(pubRaw)
	if block == nil {
		return errors.New("invalid examiner public key")
	}
	v, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return err
	}
	pub, ok := v.(ed25519.PublicKey)
	if !ok {
		return errors.New("examiner public key is not Ed25519")
	}
	pubHash := sha256.Sum256(pub)
	if idx.SigningKeyID != "sha256:"+hex.EncodeToString(pubHash[:]) {
		return errors.New("evidence index signing key ID does not match the trusted public key")
	}
	if !ed25519.Verify(pub, canonical, sig) {
		return errors.New("evidence index signature invalid")
	}
	actualFiles, err := hashTree(caseDir)
	if err != nil {
		return err
	}
	if len(actualFiles) != len(idx.Files) {
		return fmt.Errorf("evidence tree contains %d file(s), signed index contains %d", len(actualFiles), len(idx.Files))
	}
	for i, expected := range idx.Files {
		actual := actualFiles[i]
		if expected.Path != actual.Path || expected.Size != actual.Size ||
			!strings.EqualFold(expected.SHA256, actual.SHA256) {
			return fmt.Errorf("evidence file mismatch: expected %s, found %s", expected.Path, actual.Path)
		}
	}
	return nil
}

func loadSigningKey(path string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, nil, errors.New("signing key permissions are too broad; require 0600 or stricter")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, nil, errors.New("signing key is not PEM")
	}
	v, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, ok := v.(ed25519.PrivateKey)
	if !ok {
		return nil, nil, errors.New("signing key is not Ed25519")
	}
	return key, key.Public().(ed25519.PublicKey), nil
}

func hashTree(root string) ([]evidence.FileHash, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("evidence tree contains a non-regular file: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch rel {
		case "evidence-index.json", "evidence-index.sig", "examiner-public-key.pem":
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := make([]evidence.FileHash, 0, len(paths))
	for _, rel := range paths {
		sum, size, err := hashOne(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		out = append(out, evidence.FileHash{Path: rel, Size: size, SHA256: sum})
	}
	return out, nil
}

func hashOne(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func reportLines(data CaseReport) []string {
	lines := []string{
		"İMAJER - UZAK ADLİ İMAJ ALMA RAPORU",
		"",
		"Oluşturma (UTC): " + data.GeneratedAt.Format(time.RFC3339Nano),
		"Vaka ID: " + data.Case.ID,
		"Delil ID: " + data.Case.EvidenceID,
		"İncelemeci: " + data.Case.Examiner,
		"Kurum: " + data.Case.Organization,
		"Yetki referansı: " + data.Case.AuthorityRef,
		"Edinim profili: " + data.Profile,
		"",
		"HEDEF",
		"Hostname: " + data.Target.Hostname,
		"OS/Mimari: " + data.Target.OS + "/" + data.Target.Arch,
		"Kernel/Build: " + data.Target.Kernel,
		"Hedef saati (UTC): " + data.Target.UTC.Format(time.RFC3339Nano),
		"Yönetici yetkisi: " + strconv.FormatBool(data.Target.Admin),
		"Fiziksel RAM: " + strconv.FormatInt(data.Target.MemoryBytes, 10) + " byte",
		"Yerel kanıt dosya sistemi: " + data.LocalStorage.FileSystem,
		"Yerel boş alan: " + strconv.FormatUint(data.LocalStorage.Available, 10) + " byte",
	}
	if len(data.Target.Storage) > 0 {
		lines = append(lines, "Hedef depolama özeti:")
		summary := storageSummary(data.Target.Storage)
		if len(summary) == 0 {
			lines = append(lines, "  Envanter okunamadı; ham kayıt target-probe.json dosyasındadır.")
		} else {
			for _, item := range summary {
				lines = appendWrapped(lines, "  "+item, 105)
			}
			lines = append(lines, "  Ayrıntılı ham envanter: target-probe.json")
		}
	}
	lines = append(lines, "", "ARAÇ ENVANTERİ")
	toolNames := make([]string, 0, len(data.Target.Tools))
	for name := range data.Target.Tools {
		toolNames = append(toolNames, name)
	}
	sort.Strings(toolNames)
	for _, name := range toolNames {
		tool := data.Target.Tools[name]
		lines = appendWrapped(lines, "Target tool: "+name+" | "+tool.Version+" | "+tool.Path, 105)
	}
	for _, tool := range data.Tools {
		lines = appendWrapped(lines,
			fmt.Sprintf("Verified tool: %s %s %s/%s SHA-256=%s license=%s path=%s trust=%s",
				tool.Name, tool.Version, tool.OS, tool.Arch, tool.SHA256, tool.License, tool.RemotePath, tool.Trust),
			105,
		)
	}
	lines = append(lines, "", "KANITLAR")
	for _, a := range data.Artifacts {
		lines = append(lines,
			fmt.Sprintf("%s (%s): %s", a.ArtifactID, a.Kind, a.Status),
			"  Kaynak ID: "+a.SourceID,
			"  Kaynak path: "+a.SourcePath,
			"  Kaynak model: "+a.SourceModel,
			"  Kaynak/sektör boyutu: "+strconv.FormatInt(a.SourceSize, 10)+"/"+strconv.FormatInt(a.SectorSize, 10)+" byte",
			"  Boyut: "+strconv.FormatInt(a.NextOffset, 10)+" byte",
			"  Başlangıç (UTC): "+a.StartedAt.Format(time.RFC3339Nano),
			"  Bitiş (UTC): "+a.CompletedAt.Format(time.RFC3339Nano),
			"  Doğrulama seviyesi: "+a.Verification,
			"  Logical SHA-256: "+a.LogicalSHA256,
			"  Chunk Merkle root: "+a.MerkleRoot,
			"  Oturum sayısı: "+strconv.Itoa(a.SessionCount),
			"  Retry sayısı: "+strconv.Itoa(a.RetryCount),
			"  Provider: "+strings.Join(a.Providers, ", "),
		)
		if a.Resumed {
			lines = append(lines, "  UYARI: Canlı disk farklı zamanlarda okunan parçalardan oluşan bileşik imajdır.")
		}
		for _, session := range data.Sessions[a.ArtifactID] {
			lines = append(lines,
				"  Oturum: "+session.ID,
				"    Zaman: "+session.StartedAt.Format(time.RFC3339Nano)+" - "+session.EndedAt.Format(time.RFC3339Nano),
				"    Ofset: "+strconv.FormatInt(session.StartOffset, 10)+" - "+strconv.FormatInt(session.EndOffset, 10),
				"    Uzak SHA-256: "+session.RemoteSHA256,
				"    Yerel SHA-256: "+session.LocalSHA256,
			)
			if session.Error != "" {
				lines = appendWrapped(lines, "    Kesinti/hata: "+session.Error, 105)
			}
		}
	}
	lines = append(lines, "", "HEDEF FOOTPRINT")
	lines = append(lines, data.Footprint...)
	lines = append(lines, "", "UYARILAR")
	lines = append(lines, data.Warnings...)
	lines = append(lines,
		"",
		"Bütünlük ayrıntıları canonical JSON manifest ve events.jsonl dosyalarında yer alır.",
		"Zero disk footprint, hedefte imaj/staging verisi oluşturulmadığını ifade eder;",
		"geçici agent/araç dosyaları ve işletim sistemi audit kayıtları raporlanan yan etkilerdir.",
	)
	for i := range lines {
		lines[i] = sanitizeReportText(lines[i])
	}
	return lines
}

func appendWrapped(lines []string, value string, width int) []string {
	value = sanitizeReportText(value)
	for len([]rune(value)) > width {
		runes := []rune(value)
		cut := width
		for i := width; i > 0; i-- {
			if runes[i] == ' ' {
				cut = i
				break
			}
		}
		lines = append(lines, string(runes[:cut]))
		value = strings.TrimSpace(string(runes[cut:]))
	}
	return append(lines, value)
}

func storageSummary(raw json.RawMessage) []string {
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil
	}

	if devices, ok := objectList(valueByName(root, "blockdevices")); ok {
		var out []string
		for _, device := range devices {
			kind := textValue(valueByName(device, "type"))
			if kind != "" && kind != "disk" {
				continue
			}
			name := firstText(device, "path", "name", "kname")
			if name != "" && !strings.HasPrefix(name, "/") {
				name = "/dev/" + name
			}
			out = append(out, storageDeviceLine(name, device))
		}
		return out
	}

	if disks, ok := objectList(valueByName(root, "disks")); ok {
		out := make([]string, 0, len(disks))
		for _, disk := range disks {
			out = append(out, storageDeviceLine(firstText(disk, "DeviceID", "Path", "Name"), disk))
		}
		return out
	}
	return nil
}

func objectList(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				out = append(out, object)
			}
		}
		return out, true
	case map[string]any:
		return []map[string]any{typed}, true
	default:
		return nil, false
	}
}

func valueByName(object map[string]any, name string) any {
	for key, value := range object {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return nil
}

func firstText(object map[string]any, names ...string) string {
	for _, name := range names {
		if value := textValue(valueByName(object, name)); value != "" {
			return value
		}
	}
	return ""
}

func textValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}

func storageDeviceLine(name string, device map[string]any) string {
	if name == "" {
		name = "Bilinmeyen aygıt"
	}
	parts := []string{name}
	if size := textValue(valueByName(device, "size")); size != "" {
		parts = append(parts, "boyut="+formatByteCount(size))
	}
	if model := firstText(device, "model"); model != "" {
		parts = append(parts, "model="+model)
	}
	if serial := firstText(device, "serial", "SerialNumber"); serial != "" {
		parts = append(parts, "seri="+serial)
	}
	if wwn := firstText(device, "wwn"); wwn != "" {
		parts = append(parts, "WWN="+wwn)
	}
	return strings.Join(parts, " | ")
}

func formatByteCount(raw string) string {
	size, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return raw + " byte"
	}
	const unit = uint64(1024)
	if size < unit {
		return strconv.FormatUint(size, 10) + " byte"
	}
	div, exp := unit, 0
	for n := size / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB (%d byte)", float64(size)/float64(div), "KMGTPE"[exp], size)
}

func sanitizeReportText(value string) string {
	var out strings.Builder
	spacePending := false
	for _, r := range strings.ToValidUTF8(value, "�") {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 || r == 0x7f {
			spacePending = out.Len() > 0
			continue
		}
		if spacePending && r != ' ' {
			out.WriteByte(' ')
		}
		spacePending = false
		out.WriteRune(r)
	}
	return strings.TrimSpace(out.String())
}

func writePDF(path string, lines []string) error {
	const perPage = 48
	pages := (len(lines) + perPage - 1) / perPage
	if pages == 0 {
		pages = 1
	}
	fontObj := 3 + pages*2
	objects := make([][]byte, fontObj)
	objects[0] = []byte("<< /Type /Catalog /Pages 2 0 R >>")
	var kids strings.Builder
	for p := 0; p < pages; p++ {
		pageObj := 3 + p*2
		contentObj := pageObj + 1
		fmt.Fprintf(&kids, "%d 0 R ", pageObj)
		objects[pageObj-1] = []byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", fontObj, contentObj))
		start, end := p*perPage, (p+1)*perPage
		if end > len(lines) {
			end = len(lines)
		}
		var stream strings.Builder
		stream.WriteString("BT /F1 9 Tf 40 802 Td 14 TL ")
		for _, line := range lines[start:end] {
			stream.WriteString("(" + pdfEscape(line) + ") Tj T* ")
		}
		stream.WriteString("ET")
		content := stream.String()
		objects[contentObj-1] = []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}
	objects[1] = []byte(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", pages, kids.String()))
	objects[fontObj-1] = []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding << /Type /Encoding /BaseEncoding /WinAnsiEncoding /Differences [128 /Gbreve /gbreve /Idotaccent /dotlessi /Scedilla /scedilla] >> >>")

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return evidence.AtomicWrite(path, out.Bytes(), 0o600)
}

func pdfEscape(s string) string {
	var out strings.Builder
	for _, r := range sanitizeReportText(s) {
		var encoded byte
		switch r {
		case 'Ğ':
			encoded = 128
		case 'ğ':
			encoded = 129
		case 'İ':
			encoded = 130
		case 'ı':
			encoded = 131
		case 'Ş':
			encoded = 132
		case 'ş':
			encoded = 133
		default:
			if r <= 0xff {
				encoded = byte(r)
			} else {
				encoded = '?'
			}
		}
		switch encoded {
		case '\\', '(', ')':
			out.WriteByte('\\')
			out.WriteByte(encoded)
		default:
			out.WriteByte(encoded)
		}
	}
	return out.String()
}
