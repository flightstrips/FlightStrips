package services

type testStripValidationPublisher struct{}

func (testStripValidationPublisher) SendStripUpdate(int32, string) {}

func newTestStripValidationService(stripReader validationStripReader, validationStore StripValidationStatusStore) *StripValidationService {
	return &StripValidationService{
		stripReader:     stripReader,
		validationStore: validationStore,
		publisher:       testStripValidationPublisher{},
	}
}
