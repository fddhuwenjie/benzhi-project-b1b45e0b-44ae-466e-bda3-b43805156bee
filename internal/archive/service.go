package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
)

type cachedDocument struct {
	fileDigest [sha256.Size]byte
	document   Document
}

type Service struct {
	store *storage.Store
	cache sync.Map
}

func New(store *storage.Store) *Service { return &Service{store: store} }

func (s *Service) Build(caseID string) (Document, []byte, error) {
	agg, events, err := s.store.Load(caseID)
	if err != nil {
		return Document{}, nil, err
	}
	if agg.Case.Status != domain.StatusArchived && agg.Case.Status != domain.StatusDecided {
		return Document{}, nil, &domain.RuleError{Code: domain.CodeStateConflict, Message: "仅终局案件可生成档案"}
	}
	doc := Document{FormatVersion: FormatVersion, Case: agg.Case, EventRootDigest: root(events), GeneratedAt: agg.Case.ArchivedAt}
	if doc.GeneratedAt.IsZero() && len(events) > 0 {
		doc.GeneratedAt = events[len(events)-1].OccurredAt
	}
	for _, id := range keys(agg.Samples) {
		doc.Samples = append(doc.Samples, agg.Samples[id])
	}
	for _, id := range keys(agg.Evidence) {
		doc.Evidence = append(doc.Evidence, agg.Evidence[id])
	}
	for _, id := range keys(agg.Hypotheses) {
		doc.Hypotheses = append(doc.Hypotheses, agg.Hypotheses[id])
	}
	sort.Slice(doc.Hypotheses, func(i, j int) bool {
		if doc.Hypotheses[i].Rank != doc.Hypotheses[j].Rank {
			return doc.Hypotheses[i].Rank < doc.Hypotheses[j].Rank
		}
		return doc.Hypotheses[i].HypothesisID < doc.Hypotheses[j].HypothesisID
	})
	for _, id := range keys(agg.Actions) {
		doc.Actions = append(doc.Actions, agg.Actions[id])
	}
	doc.Reviews = append([]domain.ReviewDecision(nil), agg.Reviews...)
	for _, id := range keys(agg.CorrectiveItems) {
		doc.CorrectiveItems = append(doc.CorrectiveItems, agg.CorrectiveItems[id])
	}
	for _, event := range events {
		doc.Events = append(doc.Events, EventIndex{Revision: event.Revision, Type: event.Type, OccurredAt: event.OccurredAt, ActorID: event.ActorID, Digest: event.Digest})
	}
	doc.SectionDigests, err = sectionDigests(doc)
	if err != nil {
		return Document{}, nil, err
	}
	canonicalBytes, err := canonical(doc)
	if err != nil {
		return Document{}, nil, err
	}
	sum := sha256.Sum256(canonicalBytes)
	doc.ContentDigest = hex.EncodeToString(sum[:])
	final, err := json.MarshalIndent(doc, "", "  ")
	return doc, final, err
}

func (s *Service) Save(caseID string) (Document, error) {
	doc, data, err := s.Build(caseID)
	if err != nil {
		return Document{}, err
	}
	path := filepath.Join(s.store.Root(), "archives", caseID+".json")
	if err := writeAtomic(path, data); err != nil {
		return Document{}, err
	}
	return doc, nil
}

func (s *Service) Read(caseID string) (Document, []byte, error) {
	data, err := os.ReadFile(filepath.Join(s.store.Root(), "archives", caseID+".json"))
	if os.IsNotExist(err) {
		return Document{}, nil, storage.ErrNotFound
	}
	if err != nil {
		return Document{}, nil, err
	}
	fileDigest := sha256.Sum256(data)
	if value, ok := s.cache.Load(caseID); ok {
		cached := value.(cachedDocument)
		if cached.fileDigest == fileDigest {
			return cloneDocument(cached.document), data, nil
		}
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, nil, err
	}
	s.cache.Store(caseID, cachedDocument{fileDigest: fileDigest, document: doc})
	return cloneDocument(doc), data, nil
}

func (s *Service) Verify(caseID string) (Verification, error) {
	doc, _, err := s.Read(caseID)
	if err != nil {
		return Verification{}, err
	}
	result := Verification{CaseID: caseID, ContentDigest: doc.ContentDigest, EventRootDigest: doc.EventRootDigest}
	result.FormatValid = doc.FormatVersion == FormatVersion && doc.Case.CaseID == caseID
	result.TerminalStateValid = doc.Case.Status == domain.StatusArchived
	expectedSections := map[string]string{}
	for _, section := range doc.SectionDigests {
		expectedSections[section.Name] = section.Digest
	}
	actualSections, sectionErr := sectionDigests(doc)
	if sectionErr != nil {
		return result, sectionErr
	}
	for _, section := range actualSections {
		expected := expectedSections[section.Name]
		result.Sections = append(result.Sections, SectionVerification{Name: section.Name, Valid: expected != "" && expected == section.Digest, ExpectedDigest: expected, ActualDigest: section.Digest})
	}
	b, err := canonical(doc)
	if err != nil {
		return result, err
	}
	sum := sha256.Sum256(b)
	result.ContentDigestValid = hex.EncodeToString(sum[:]) == doc.ContentDigest
	rootDigest, err := s.store.RootDigest(caseID)
	if err != nil {
		return result, err
	}
	result.EventRootDigestValid = rootDigest == doc.EventRootDigest
	sectionsValid := len(result.Sections) == 6
	for _, section := range result.Sections {
		sectionsValid = sectionsValid && section.Valid
	}
	result.Valid = result.FormatValid && result.TerminalStateValid && result.ContentDigestValid && result.EventRootDigestValid && sectionsValid
	if result.Valid {
		result.Message = "档案六个业务分区、整档摘要与事件链均有效"
	} else {
		result.Message = "档案完整性校验失败，请依据分区结果定位受影响内容"
	}
	return result, nil
}

func cloneDocument(doc Document) Document {
	b, err := json.Marshal(doc)
	if err != nil {
		return doc
	}
	var clone Document
	if err := json.Unmarshal(b, &clone); err != nil {
		return doc
	}
	return clone
}

func sectionDigests(doc Document) ([]SectionDigest, error) {
	type dispositionEntry struct {
		SampleID             string             `json:"sample_id"`
		Disposition          domain.Disposition `json:"disposition"`
		AllowedResearchScope []string           `json:"allowed_research_scope,omitempty"`
		Reason               string             `json:"reason"`
		BasisReferences      []string           `json:"basis_references,omitempty"`
	}
	dispositions := make([]dispositionEntry, 0, len(doc.Samples))
	for _, sample := range doc.Samples {
		dispositions = append(dispositions, dispositionEntry{SampleID: sample.SampleID, Disposition: sample.Disposition, AllowedResearchScope: sample.AllowedResearchScope, Reason: sample.DispositionReason, BasisReferences: sample.DispositionBasisReferences})
	}
	sections := []struct {
		Name  string
		Value any
	}{
		{"case_samples", struct {
			Case    domain.ContaminationCase `json:"case"`
			Samples []domain.IceCoreSample   `json:"samples"`
		}{doc.Case, doc.Samples}},
		{"evidence", doc.Evidence},
		{"hypotheses", doc.Hypotheses},
		{"remediation", doc.Actions},
		{"review_dispositions", struct {
			Reviews         []domain.ReviewDecision `json:"reviews"`
			CorrectiveItems []domain.CorrectiveItem `json:"corrective_items"`
			Dispositions    []dispositionEntry      `json:"dispositions"`
		}{doc.Reviews, doc.CorrectiveItems, dispositions}},
		{"event_index", doc.Events},
	}
	result := make([]SectionDigest, 0, len(sections))
	for _, section := range sections {
		b, err := json.Marshal(section.Value)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(b)
		result = append(result, SectionDigest{Name: section.Name, Digest: hex.EncodeToString(sum[:])})
	}
	return result, nil
}

func root(events []storage.StoredEvent) string {
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1].Digest
}
func keys[T any](m map[string]T) []string {
	values := make([]string, 0, len(m))
	for k := range m {
		values = append(values, k)
	}
	sort.Strings(values)
	return values
}
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".archive-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("同步档案目录: %w", err)
	}
	return nil
}
