package events

const (
	E_COLLECTION_CREATE               = "COLLECTION_CREATE"
	E_COLLECTION_UPDATE               = "COLLECTION_UPDATE"
	E_COLLECTION_DELETE               = "COLLECTION_DELETE"
	E_COLLECTION_PUBLISHED_CHANGE     = "COLLECTION_PUBLISHED_CHANGE"
	E_COLLECTION_CONTENT_UNITS_CHANGE = "COLLECTION_CONTENT_UNITS_CHANGE"

	E_CONTENT_UNIT_CREATE             = "CONTENT_UNIT_CREATE"
	E_CONTENT_UNIT_UPDATE             = "CONTENT_UNIT_UPDATE"
	E_CONTENT_UNIT_DELETE             = "CONTENT_UNIT_DELETE"
	E_CONTENT_UNIT_PUBLISHED_CHANGE   = "CONTENT_UNIT_PUBLISHED_CHANGE"
	E_CONTENT_UNIT_DERIVATIVES_CHANGE = "CONTENT_UNIT_DERIVATIVES_CHANGE"
	E_CONTENT_UNIT_SOURCES_CHANGE     = "CONTENT_UNIT_SOURCES_CHANGE"
	E_CONTENT_UNIT_TAGS_CHANGE        = "CONTENT_UNIT_TAGS_CHANGE"
	E_CONTENT_UNIT_PERSONS_CHANGE     = "CONTENT_UNIT_PERSONS_CHANGE"
	E_CONTENT_UNIT_PUBLISHERS_CHANGE  = "CONTENT_UNIT_PUBLISHERS_CHANGE"

	E_FILE_UPDATE    = "FILE_UPDATE"
	E_FILE_PUBLISHED = "FILE_PUBLISHED"
	E_FILE_INSERT    = "FILE_INSERT"
	E_FILE_REPLACE   = "FILE_REPLACE"
	E_FILE_REMOVE    = "FILE_REMOVE"

	E_SOURCE_CREATE = "SOURCE_CREATE"
	E_SOURCE_UPDATE = "SOURCE_UPDATE"

	E_TAG_CREATE = "TAG_CREATE"
	E_TAG_UPDATE = "TAG_UPDATE"

	E_PERSON_CREATE = "PERSON_CREATE"
	E_PERSON_UPDATE = "PERSON_UPDATE"
	E_PERSON_DELETE = "PERSON_DELETE"

	E_PUBLISHER_CREATE = "PUBLISHER_CREATE"
	E_PUBLISHER_UPDATE = "PUBLISHER_UPDATE"
)
