package api

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/gin-gonic/gin.v1"

	"github.com/Bnei-Baruch/archive-backend/consts"
)

type podcastFeedXml struct {
	XMLName       xml.Name `xml:"rss"`
	Version       string   `xml:"version,attr"`
	XmlnsItunes   string   `xml:"xmlns:itunes,attr"`
	XmlnsAtom     string   `xml:"xmlns:atom,attr"`
	XmlnsRawvoice string   `xml:"xmlns:rawvoice,attr,omitempty"`

	Channel *podcastChannel
}

type podcastChannel struct {
	XMLName         xml.Name `xml:"channel"`
	Title           string   `xml:"title"`       // required
	Link            string   `xml:"link"`        // required
	Description     string   `xml:"description"` // required
	Image           *podcastImage
	Language        string `xml:"language"`
	Copyright       string `xml:"copyright"`
	PodcastAtomLink *podcastAtomLink
	LastBuildDate   string `xml:"lastBuildDate"`
	Author          string `xml:"itunes:author"`
	Summary         string `xml:"itunes:summary"`
	Subtitle        string `xml:"itunes:subtitle,omitempty"`
	Owner           *podcastOwner
	Explicit        string `xml:"itunes:explicit"`
	Keywords        string `xml:"itunes:keywords"`
	ItunesImage     *itunesImage
	Category        *podcastCategory
	PubDate         string `xml:"pubDate"`
	Items           []*podcastItem
}

type podcastItem struct {
	XMLName     xml.Name `xml:"item"`
	Title       string   `xml:"title"`             // required
	Link        string   `xml:"link"`              // required
	PubDate     string   `xml:"pubDate,omitempty"` // created or updated
	Description string   `xml:"description"`       // required
	Enclosure   *podcastEnclosure
	Guid        string `xml:"guid"`
	Duration    string `xml:"itunes:duration"`
	Summary     string `xml:"itunes:summary"`
	Image       *itunesImage
	Keywords    string `xml:"itunes:keywords,omitempty"`
	Explicit    string `xml:"itunes:explicit"`
}

type podcastEnclosure struct {
	XMLName xml.Name `xml:"enclosure"`
	Url     string   `xml:"url,attr"`
	Length  int64    `xml:"length,attr"`
	Type    string   `xml:"type,attr"`
}

type podcastCategory struct {
	XMLName  xml.Name `xml:"itunes:category"`
	Text     string   `xml:"text,attr"`
	Category *podcastCategory
}

type podcastOwner struct {
	XMLName xml.Name `xml:"itunes:owner,omitempty"`
	Name    string   `xml:"itunes:name,omitempty"`
	Email   string   `xml:"itunes:email,omitempty"`
}

type podcastImage struct {
	XMLName xml.Name `xml:"image"`
	Url     string   `xml:"url"`
	Title   string   `xml:"title"`
	Link    string   `xml:"link"`
}

type itunesImage struct {
	XMLName xml.Name `xml:"itunes:image"`
	Href    string   `xml:"href,attr"`
}

type podcastAtomLink struct {
	XMLName xml.Name `xml:"atom:link"`
	Href    string   `xml:"href,attr"`
	Rel     string   `xml:"rel,attr"`
	Type    string   `xml:"type,attr"`
}

func (c *podcastChannel) CreateFeed() interface{} {
	return &podcastFeedXml{
		Version:     "2.0",
		XmlnsItunes: "http://www.itunes.com/dtds/podcast-1.0.dtd",
		XmlnsAtom:   "http://www.w3.org/2005/Atom",
		Channel:     c,
	}
}

type translation struct {
	Title       string
	Description string
	Keywords    string
	Author      string
	A           string
	V           string
	X           string
}

var T = map[string]translation{
	consts.LANG_ENGLISH:    {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_HEBREW:     {A: "אודיו", V: "וידאו", X: "אודיו-וידאו", Title: "שיעור הקבלה היומי", Description: "במשך אלפי שנים, היו המקובלים לומדים על בסיס יומי, למען התפתחותם הרוחנית הפרטית ולמען התקדמותה הרוחנית של האנושות. בימינו, ממשיכים את אותה המסורת קבוצת המקובלים ״בני ברוך״, הלומדים מדי יום מתוך כתבי הקבלה האותנטיים, לימודים המלווים בביאור והדרכה מפי הרב ד״ר מיכאל לייטמן.", Keywords: "קבלה,שיעור,רוחניות,אותנטי", Author: "ד״ר מיכאל לייטמן"},
	consts.LANG_RUSSIAN:    {A: "Аудио", V: "Видео", X: "Аудио-Видео", Title: "Ежедневный урок по каббале", Description: "На протяжении тысячелетий каббалисты учились каждый день, ради своего личного духовного возвышения, и ради духовного возвышения человечества. В наше время продолжает эту традицию каббалистическая группа \"Бней Барух\",  занимаясь ежедневно по подлинным каббалистическим источникам, под руководством ученого – каббалиста, основателя Международной академии каббалы, Михаэля Лайтмана.", Keywords: "каббала,уроки,духовность,аутентичная", Author: "Михаэль Лайтман"},
	consts.LANG_UKRAINIAN:  {A: "Аудио", V: "Видео", X: "Аудио-Видео", Title: "Ежедневный урок по каббале (UKR)", Description: "На протяжении тысячелетий каббалисты учились каждый день, ради своего личного духовного возвышения, и ради духовного возвышения человечества. В наше время продолжает эту традицию каббалистическая группа \"Бней Барух\",  занимаясь ежедневно по подлинным каббалистическим источникам, под руководством ученого – каббалиста, основателя Международной академии каббалы, Михаэля Лайтмана.", Keywords: "каббала,уроки,духовность,аутентичная", Author: "Михаэль Лайтман"},
	consts.LANG_SPANISH:    {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (SPA)", Description: "Durante miles de años, los cabalistas se consagraron a estudiar día tras día para alcanzar el progreso espiritual de la humanidad y el suyo propio. En el Instituto Bnei Baruj para la Educación y la Investigación de la Cabalá continuamos con esta tradición en el mundo globalizado de hoy, estudiando diariamente las fuentes auténticas cabalísticas, enriquecidas con los comentarios del Rav doctor Michael Laitman", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_ITALIAN:    {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (ITA)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_GERMAN:     {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (GER)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_DUTCH:      {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (DUT)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_FRENCH:     {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (FRE)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_PORTUGUESE: {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (POR)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_TURKISH:    {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (TRK)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_POLISH:     {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (POL)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_ARABIC:     {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (ARB)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_HUNGARIAN:  {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (HUN)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_FINNISH:    {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (FIN)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_LITHUANIAN: {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (LIT)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_JAPANESE:   {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (JPN)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_BULGARIAN:  {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (BUL)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_GEORGIAN:   {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (GEO)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_NORWEGIAN:  {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (NOR)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_SWEDISH:    {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (SWE)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_CHINESE:    {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (CHN)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_PERSIAN:    {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (PER)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_ROMANIAN:   {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (RON)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_HINDI:      {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (HIN)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_MACEDONIAN: {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (MKD)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_SLOVENIAN:  {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (SLV)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_LATVIAN:    {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (LAV)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_SLOVAK:     {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (SLK)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
	consts.LANG_CZECH:      {A: "Audio", V: "Video", X: "Audio-Video", Title: "Daily Kabbalah Lesson (CZE)", Description: "For thousands of years, Kabbalists have been studying on a daily basis for their and humanity's spiritual progress. Continuing this tradition into today's globally connected world, the Bnei Baruch Kabbalah Education & Research Institute, studies daily from authentic Kabbalistic sources, with commentary by Dr. Michael Laitman.", Keywords: "kabbalah,lessons,spirituality,authentic", Author: "Dr. Michael Laitman"},
}

var CT = map[string]map[string]string{
	"content_type.DAILY_LESSON": {
		"he": "שיעור יומי",
		"en": "Daily Lesson",
		"ru": "Урок",
		"es": "Lección Diaria",
		"ua": "Урок",
		"de": "Tägliche Lektion",
		"it": "Lezione del giorno",
		"cz": "Denní lekce",
		"tr": "Günlük Ders",
	},
	"content_type.SPECIAL_LESSON": {
		"he": "שיעור יומי",
		"en": "Daily Lesson",
		"ru": "Урок",
		"es": "Lección Diaria",
		"ua": "Урок",
		"de": "Besondere Lektion",
		"it": "Lezione speciale",
		"cz": "Denní lekce",
		"tr": "Özel Ders",
	},
	"content_type.FRIENDS_GATHERINGS": {
		"he": "ישיבות חברים",
		"en": "Gatherings of Friends",
		"ru": "Ешиват хаверим",
		"es": "Asamblea de Amigos",
		"ua": "Єшиват Хаверім",
		"de": "Versammlungen der Freunde",
		"it": "Assemblea degli Amici",
		"cz": "Shromáždění přátel",
		"tr": "Dostlar Toplantısı",
	},
	"content_type.CONGRESS": {
		"he": "כנס",
		"en": "Convention",
		"ru": "Конгресс",
		"es": "Congreso",
		"ua": "Конгрес",
		"de": "Kongress",
		"it": "Congresso",
		"cz": "Setkání",
		"tr": "Kongre",
	},
	"content_type.VIDEO_PROGRAM": {
		"he": "תוכנית טלוויזיה",
		"en": "TV Show",
		"ru": "ТВ программа",
		"es": "Programa",
		"ua": "ТВ Програма",
		"de": "Videoprogramm",
		"it": "Programma video",
		"cz": "TV pořad",
		"tr": "TV Programı",
	},
	"content_type.LECTURE_SERIES": {
		"he": "סדרת הרצאות",
		"en": "Lecture Series",
		"ru": "Серия лекций",
		"es": "Serie de Charlas",
		"ua": "Серія Лекцій",
		"de": "Vortragsreihe",
		"it": "Serie di conferenze",
		"cz": "Série přednášek",
		"tr": "Konferans Serileri",
	},
	"content_type.CHILDREN_LESSONS": {
		"he": "שיעורי ילדים",
		"en": "Children Lessons",
		"ru": "Детские уроки",
		"es": "Lección de Niños",
		"ua": "Діти Уроки",
		"de": "Unterrichte für Kinder",
		"it": "Lezioni per i bambini",
		"cz": "Dětské lekce",
		"tr": "Çocuk Dersleri",
	},
	"content_type.WOMEN_LESSONS": {
		"he": "שיעורי נשים",
		"en": "Women Lessons",
		"ru": "Женские уроки",
		"es": "Lección de Mujeres",
		"ua": "Уроки для жiнок",
		"de": "Unterrichte für Frauen",
		"it": "Lezioni per le donne",
		"cs": "Ženské lekce",
		"tr": "Kadın Dersleri",
	},
	"content_type.VIRTUAL_LESSONS": {
		"he": "שיעורים וירטואלים",
		"en": "Virtual Lessons",
		"ru": "Виртуальные уроки",
		"es": "Lecciones Virtuales",
		"ua": "Віртуальнi Уроки",
		"de": "Virtuelle Lektionen",
		"it": "Lezioni virtuali",
		"cz": "Virtuální lekce",
		"tr": "Sanal Dersler",
	},
	"content_type.MEALS": {
		"he": "סעודות",
		"en": "Meals",
		"ru": "Трапезы",
		"es": "Comidas",
		"ua": "Трапези",
		"de": "Mahlzeiten",
		"it": "Pasti",
		"cs": "Jídla",
		"tr": "Yemekler",
	},
	"content_type.HOLIDAY": {
		"he": "חג",
		"en": "Holiday",
		"ru": "Праздник",
		"es": "Fiestas",
		"ua": "Свято",
		"de": "Feiertag",
		"it": "Festività",
		"cs": "Svátky",
		"tr": "Bayram",
	},
	"content_type.PICNIC": {
		"he": "פיקניק",
		"en": "Picnic",
		"ru": "Пикник",
		"es": "Picnic",
		"ua": "Пікнік",
		"de": "Picknick",
		"it": "Picnic",
		"cs": "Piknik",
		"tr": "Piknik",
	},
	"content_type.UNITY_DAY": {
		"he": "יום איחוד",
		"en": "Unity Day",
		"ru": "День народного единства",
		"es": "Día de Unión",
		"ua": "День Народної Єдності",
		"de": "Unity Day",
		"it": "Unity Day- Giorno dell’Unione",
		"cs": "Den jednoty",
		"tr": "Birlik Günü",
	},
	"content_type.CLIPS": {
		"he": "קליפים",
		"en": "Clips",
		"ru": "Клипы",
		"es": "Clips",
		"ua": "Кліпи",
		"de": "Clips",
		"it": "Video brevi",
		"cs": "Klip",
		"tr": "Klipler",
	},
	"content_type.ARTICLES": {
		"he": "מאמרים",
		"en": "Articles",
		"ru": "Статьи",
		"es": "Artículos",
		"ua": "Статті",
		"de": "Artikel",
		"it": "Articoli",
		"cs": "Články",
		"tr": "Makaleler",
	},
	"content_type.LESSONS_SERIES": {
		"he": "סדרת שיעורים",
		"en": "Lessons Series",
		"ru": "Серия уроки",
		"es": "Serie de Lecciones",
		"ua": "Серія Уроки",
		"de": "Teil Nr. der Lektion",
		"it": "Serie di lezioni",
		"cs": "Série lekcí",
		"tr": "Ders Serileri",
	},
	"content_type.SONGS": {
		"he": "שירים",
		"en": "Songs",
		"ru": "песни",
		"es": "Canciones",
		"ua": "Пісні",
		"de": "Lieder",
		"it": "Canzoni",
		"cs": "Písnišky",
		"tr": "Şarkılar",
	},
	"content_type.BOOKS": {
		"he": "ספרים",
		"en": "Books",
		"ru": "книги",
		"es": "Libros",
		"ua": "Книги",
		"de": "Bücher",
		"it": "Libri",
		"cs": "Knihy",
		"tr": "Kitaplar",
	},
	"content_type.LESSON_PART": {
		"he": "חלק שיעור",
		"en": "Lesson Part",
		"ru": "Урок часть",
		"es": "Parte de la Lección",
		"ua": "Урок Частина",
		"de": "Lektionsteil",
		"it": "Parte dalla lezione",
		"cs": "Část lekce",
		"tr": "Ders Bölümü",
	},
	"content_type.LECTURE": {
		"he": "הרצאה",
		"en": "Lecture",
		"ru": "Лекция",
		"es": "Charla",
		"ua": "Лекція",
		"de": "Lektion",
		"it": "Conferenza",
		"cs": "Přednáška",
		"tr": "Konferans",
	},
	"content_type.CHILDREN_LESSON": {
		"he": "שיעור ילדים",
		"en": "Children Lesson",
		"ru": "Детский урок",
		"es": "Lección de Niños",
		"ua": "Діти Урок",
		"de": "Unterricht für Kinder",
		"it": "Lezione per i bambini",
		"cs": "Dětská lekce",
		"tr": "Çocuk Dersi",
	},
	"content_type.WOMEN_LESSON": {
		"he": "שיעור נשים",
		"en": "Women Lesson",
		"ru": "Женский урок",
		"es": "Lección de Mujeres",
		"ua": "Урок для жiнок",
		"de": "Unterricht für Frauen",
		"it": "Lezione per le donne",
		"cs": "Ženská lekce",
		"tr": "Kadın Dersi",
	},
	"content_type.VIRTUAL_LESSON": {
		"he": "שיעור וירטואלי",
		"en": "Virtual Lesson",
		"ru": "Виртуальный урок",
		"es": "Lección Virtual",
		"ua": "Віртуальний Урок",
		"de": "Virtuelle Lektion",
		"it": "Lezione virtuale",
		"cs": "Virtuální lekce",
		"tr": "Sanal Ders",
	},
	"content_type.FRIENDS_GATHERING": {
		"he": "ישיבת חברים",
		"en": "Gathering of Friends",
		"ru": "Ешиват хаверим",
		"es": "Asamblea de Amigos",
		"ua": "Єшиват Хаверім",
		"de": "Versammlung der Freunde",
		"it": "Assemblea degli Amici",
		"cs": "Setkávání přátel",
		"tr": "Dostlar Toplantısı",
	},
	"content_type.MEAL": {
		"he": "סעודה",
		"en": "Meal",
		"ru": "Трапеза",
		"es": "Comida",
		"ua": "Трапеза",
		"de": "Mahlzeit",
		"it": "Assemblea degli Amici",
		"cs": "Jídlo",
		"tr": "Yemek",
	},
	"content_type.VIDEO_PROGRAM_CHAPTER": {
		"he": "פרק",
		"en": "Episode",
		"ru": "Эпизод",
		"es": "Capitulo",
		"ua": "Епізод",
		"de": "Teil des Programmes",
		"it": "Episodio del programma video",
		"cs": "Epizoda",
		"tr": "Bölüm",
	},
	"content_type.FULL_LESSON": {
		"he": "שיעור מלא",
		"en": "Full Lesson",
		"ru": "Полный урок",
		"es": "Lección Completa",
		"ua": "Цілий Урок",
		"de": "Vollständige Lektion",
		"it": "Lezione completa",
		"cs": "Celá lekce",
		"tr": "Tam Ders",
	},
	"content_type.ARTICLE": {
		"he": "מאמר",
		"en": "Article",
		"ru": "Статья",
		"es": "Articulo",
		"ua": "Стаття",
		"de": "Artikel",
		"it": "Articolo",
		"cs": "Článek",
		"tr": "Makale",
	},
	"content_type.UNKNOWN": {
		"he": "לא ידוע",
		"en": "Unknown",
		"ru": "Неизвестный",
		"es": "Desconocido",
		"ua": "Невідомий",
		"de": "Unbekannt",
		"it": "Sconosciuto",
		"cs": "Neznámé",
		"tr": "Bilinmeyen",
	},
	"content_type.EVENT_PART": {
		"he": "חלק מאירוע",
		"en": "Event Part",
		"ru": "Событие часть",
		"es": "Parte del Evento",
		"ua": "Подія Частина",
		"de": "Teil der Veranstaltung",
		"it": "Parte dell'evento",
		"cs": "Část události",
		"tr": "Olaylar Bölümü",
	},
	"content_type.CLIP": {
		"he": "קליפ",
		"en": "Clip",
		"ru": "Клип",
		"es": "Clip",
		"ua": "Кліп",
		"de": "Clip",
		"it": "Video breve",
		"cs": "Klip",
		"tr": "Klip",
	},
	"content_type.TRAINING": {
		"he": "הכשרה",
		"en": "Training",
		"ru": "Обучение",
		"es": "Entrenamiento",
		"ua": "Навчання",
		"de": "Training",
		"it": "Formazione",
		"cs": "Trénink",
		"tr": "Eğitim",
	},
	"content_type.KITEI_MAKOR": {
		"he": "קטעי מקור",
		"en": "Source Excerpts",
		"ru": "Исходные выдержки",
		"es": "Extractos de Fuentes",
		"ua": "Початкові Витяги",
		"de": "Fragmente von Primärquellen",
		"it": "Estratti dalle fonti",
		"cs": "Výňatky ze zdrojů",
		"tr": "Kaynaklardan Alıntılar",
	},
	"content_type.PUBLICATION": {
		"he": "פירסום",
		"en": "Publication",
		"ru": "Публикация",
		"es": "Publicidad",
		"ua": "Публікація",
		"de": "Veröffentlichung",
		"it": "Pubblicazione",
		"cs": "Publikace",
		"tr": "Yayınlar",
	},
	"content_type.LELO_MIKUD": {
		"he": "ללא קבוצת מיקוד",
		"en": "Without Focus Group",
		"ru": "Без фокус-группы",
		"es": "Editado - sin grupo de enfoque",
		"ua": "Без Фокус-Групи",
		"de": "Bearbeitet - Ohne Fokusgruppen",
		"it": "Senza Focus Group",
		"cs": "Bez Focus Group",
		"tr": "Fokuz Grupsuz",
	},
	"content_type.SONG": {
		"he": "שיר",
		"en": "Song",
		"ru": "песня",
		"es": "Canción",
		"ua": "Пісня",
		"de": "Lied",
		"it": "Canzone",
		"cs": "Píseň",
		"tr": "Şarkı",
	},
	"content_type.BOOK": {
		"he": "ספר",
		"en": "Book",
		"ru": "Книга",
		"es": "Libro",
		"ua": "Книга",
		"de": "Buch",
		"it": "Libro",
		"cs": "Kniha",
		"tr": "Kitap",
	},
	"content_type.BLOG_POST": {
		"he": "פוסט בבלוג",
		"en": "Blog Post",
		"ru": "Сообщение блога",
		"es": "Entrada en el blog",
		"ua": "Публікація блогів",
		"de": "Blogeintrag",
		"it": "Post dal Blog",
		"cs": "Blog Post",
		"tr": "Blok Postu",
	},
	"content_type.RESEARCH_MATERIAL": {
		"he": "חומרי מחקר",
		"en": "Research Material",
		"ru": "Материал исследования",
		"es": "Material de investigación",
		"ua": "Матеріал дослідження",
		"de": "Forschungsmaterial",
		"it": "Materiale di ricerca",
		"cs": "Výzkumný materiál",
		"tr": "Araştırma Materyali",
	},
	"content_type.KTAIM_NIVCHARIM": {
		"he": "קטעים נבחרים",
		"en": "Selected Highlights",
		"ru": "Избранные моменты",
		"es": "Destacados Seleccionados",
		"ua": "Вибрані моменти",
		"de": "Ausgewählte Highlights",
		"it": "Punti Salienti Selezionati",
		"cs": "Vybraná zvýraznění",
		"tr": "Seçilen Önemli Noktalar",
	},
}
func FeedPodcast(c *gin.Context) {
	var config feedConfig
	(&config).getConfig(c)

	t := T[config.Lang]

	//DF=[A]/V
	var mediaTypes []string
	var mediaType string
	if config.DF == "V" {
		mediaTypes = []string{consts.MEDIA_MP4}
		mediaType = t.V
	} else if config.DF == "A" {
		mediaTypes = []string{consts.MEDIA_MP3a, consts.MEDIA_MP3b}
		mediaType = t.A
	} else {
		mediaTypes = []string{consts.MEDIA_MP4, consts.MEDIA_MP3a, consts.MEDIA_MP3b}
		mediaType = t.X
	}

	title := t.Title + " (" + mediaType + ")"
	description := t.Description + " (" + mediaType + ")"
	href := "https://old.kabbalahmedia.info/cover_podcast.jpg"
	link := getHref("/feeds/podcast/"+config.DLANG+"/"+config.DF, c)

	channel := &podcastChannel{
		Title:           title,
		Link:            link,
		Description:     description,
		Image:           &podcastImage{Url: href, Title: title, Link: link},
		Language:        config.Lang,
		Copyright:       copyright,
		PodcastAtomLink: &podcastAtomLink{Href: link, Rel: "self", Type: "application/rss+xml"},
		LastBuildDate:   time.Now().Format(time.RFC1123), // TODO: get a newest created_at of files
		Author:          t.Author,
		Summary:         description,
		Subtitle:        "",
		Owner:           &podcastOwner{Name: "Bnei Baruch Association", Email: "info@kab.co.il"},
		Explicit:        "no",
		Keywords:        t.Keywords,
		ItunesImage:     &itunesImage{Href: href},
		Category:        &podcastCategory{Text: "Religion & Spirituality", Category: &podcastCategory{Text: "Spirituality"}},
		PubDate:         time.Now().Format(time.RFC1123),

		Items: make([]*podcastItem, 0),
	}

	db := c.MustGet("MDB_DB").(*sql.DB)
	cur := ContentUnitsRequest{
		ListRequest: ListRequest{
			BaseRequest: BaseRequest{
				Language: config.Lang,
			},
			StartIndex: 1,
			StopIndex:  20,
			OrderBy:    "created_at desc",
		},
		ContentTypesFilter: ContentTypesFilter{
			ContentTypes: []string{consts.CT_LESSON_PART},
		},
	}

	languages := []string{config.Lang}
	item, err := handleContentUnitsFull(db, cur, mediaTypes, languages)
	if err != nil {
		if err == sql.ErrNoRows {
			c.XML(http.StatusOK, channel.CreateFeed())
		} else {
			NewInternalError(err).Abort(c)
		}
		return
	}

	var nameToIgnore = regexp.MustCompile("kitei-makor|lelo-mikud")
	for _, cu := range item.ContentUnits {
		files := cu.Files
		for _, file := range files {
			if nameToIgnore.MatchString(file.Name) {
				continue
			}

			// TODO: change title and description
			url := fmt.Sprintf("%s%s%s", consts.CDN, file.ID, filepath.Ext(file.Name))
			description := cu.Description
			if description == "" {
				description = cu.Name
			}
			channel.Items = append(channel.Items, &podcastItem{
				Title:       file.CreatedAt.Format(time.RFC822) + "; " + cu.Name,
				Link:        url,
				PubDate:     file.CreatedAt.Format(time.RFC822),
				Description: description,
				Enclosure: &podcastEnclosure{
					Url:    url,
					Length: file.Size,
					Type:   consts.MEDIA_MP3,
				},
				Guid:     url,
				Duration: convertDuration(cu.Duration),
				Summary:  description,
				Image:    &itunesImage{Href: href},
				Keywords: t.Keywords,
				Explicit: "no",
			})
		}
	}

	feed := channel.CreateFeed()
	feedXml, err := xml.Marshal(feed)
	payload := []byte(xml.Header + string(feedXml))
	c.Data(http.StatusOK, "application/xml; charset=utf-8", payload)
}

func FeedCollections(c *gin.Context) {
	var config feedConfig
	(&config).getConfig(c)

	db := c.MustGet("MDB_DB").(*sql.DB)

	//DF=[A]/V/X
	var mediaTypes []string
	if config.DF == "V" {
		mediaTypes = []string{consts.MEDIA_MP4}
	} else if config.DF == "A" {
		mediaTypes = []string{consts.MEDIA_MP3a, consts.MEDIA_MP3b}
	} else {
		mediaTypes = []string{consts.MEDIA_MP4, consts.MEDIA_MP3a, consts.MEDIA_MP3b}
	}

	href := "https://old.kabbalahmedia.info/cover_podcast.jpg"
	l := "/feeds/collections/" + config.DLANG + "/" + config.COLLECTION
	if config.DF != "" {
		l += "/df/" + config.DF
	}
	if config.TAG != "" {
		l += "/tag/" + config.TAG
	}
	link := getHref(l, c)
	r := BaseRequest{
		Language: config.Lang,
	}
	collection, errH := handleCollectionWOCUs(db, ItemRequest{
		BaseRequest: r,
		UID:         config.COLLECTION,
	})
	if errH != nil {
		errH.Abort(c)
		return
	}
	title := collection.Name
	description := collection.Description
	channel := &podcastChannel{
		Title:           title,
		Link:            link,
		Description:     description,
		Image:           &podcastImage{Url: href, Title: title, Link: link},
		Language:        config.Lang,
		Copyright:       copyright,
		PodcastAtomLink: &podcastAtomLink{Href: link, Rel: "self", Type: "application/rss+xml"},
		LastBuildDate:   time.Now().Format(time.RFC1123),
		Author:          T[config.Lang].Author,
		Summary:         description,
		Subtitle:        "",
		Owner:           &podcastOwner{Name: "Bnei Baruch Association", Email: "info@kab.co.il"},
		Explicit:        "no",
		Keywords:        "",
		ItunesImage:     &itunesImage{Href: href},
		Category:        &podcastCategory{Text: "Religion & Spirituality", Category: &podcastCategory{Text: "Spirituality"}},
		PubDate:         time.Now().Format(time.RFC1123),

		Items: make([]*podcastItem, 0),
	}

	cur := ContentUnitsRequest{
		ListRequest: ListRequest{
			BaseRequest: BaseRequest{
				Language: config.Lang,
			},
			StartIndex: 1,
			StopIndex:  20,
			OrderBy:    "created_at desc",
		},
		CollectionsFilter: CollectionsFilter{
			Collections: []string{config.COLLECTION},
		},
		WithTags: true,
	}
	if config.TAG != "" {
		cur.Tags = []string{config.TAG}
	}
	languages := []string{config.Lang}
	renderContentUnits(db, cur, mediaTypes, languages, c, channel, href)
}

func FeedByContentType(c *gin.Context) {
	var config feedConfig
	(&config).getConfig(c)

	db := c.MustGet("MDB_DB").(*sql.DB)

	//DF=[A]/V/X
	var mediaTypes []string
	if config.DF == "V" {
		mediaTypes = []string{consts.MEDIA_MP4}
	} else if config.DF == "A" {
		mediaTypes = []string{consts.MEDIA_MP3a, consts.MEDIA_MP3b}
	} else {
		mediaTypes = []string{consts.MEDIA_MP4, consts.MEDIA_MP3a, consts.MEDIA_MP3b}
	}
	href := "https://old.kabbalahmedia.info/cover_podcast.jpg"
	l := "/feeds/content_type/" + config.DLANG + "/" + strings.Join(config.CT, ",")
	if len(config.TAG) > 0 {
		l += "/tag/" + config.TAG
	}
	if config.DF != "" {
		l += "/df/" + config.DF
	}
	r := BaseRequest{
		Language: config.Lang,
	}
	tags, errH := handleTagsTranslationByID(db, r, []string{config.TAG})
	if errH != nil {
		errH.Abort(c)
		return
	}

	title := ""
	if ct, ok := CT["content_type."+config.CT[0]]; ok {
		if tr, ok := ct[config.Lang]; ok {
			title = title + tr
		}
	}
	if len(tags) > 0 {
		if len(title) > 0 {
			title += "; "
		}
		title += strings.Join(tags, ", ")
	}
	description := title
	link := getHref(l, c)
	channel := &podcastChannel{
		Title:           title,
		Link:            link,
		Description:     description,
		Image:           &podcastImage{Url: href, Title: title, Link: link},
		Language:        config.Lang,
		Copyright:       copyright,
		PodcastAtomLink: &podcastAtomLink{Href: link, Rel: "self", Type: "application/rss+xml"},
		LastBuildDate:   time.Now().Format(time.RFC1123),
		Author:          T[config.Lang].Author,
		Summary:         description,
		Subtitle:        "",
		Owner:           &podcastOwner{Name: "Bnei Baruch Association", Email: "info@kab.co.il"},
		Explicit:        "no",
		Keywords:        "",
		ItunesImage:     &itunesImage{Href: href},
		Category:        &podcastCategory{Text: "Religion & Spirituality", Category: &podcastCategory{Text: "Spirituality"}},
		PubDate:         time.Now().Format(time.RFC1123),

		Items: make([]*podcastItem, 0),
	}

	cur := ContentUnitsRequest{
		ListRequest: ListRequest{
			BaseRequest: BaseRequest{
				Language: config.Lang,
			},
			StartIndex: 1,
			StopIndex:  20,
			OrderBy:    "created_at desc",
		},
		ContentTypesFilter: ContentTypesFilter{
			ContentTypes: config.CT,
		},
		WithTags: true,
	}
	if config.TAG != "" {
		cur.TagsFilter = TagsFilter{
			Tags: []string{config.TAG},
		}
	}
	languages := []string{config.Lang}
	renderContentUnits(db, cur, mediaTypes, languages, c, channel, href)
}

func renderContentUnits(db *sql.DB, cur ContentUnitsRequest, mediaTypes []string, languages []string, c *gin.Context, channel *podcastChannel, href string) {
	item, err := handleContentUnitsFull(db, cur, mediaTypes, languages)
	if err != nil {
		if err == sql.ErrNoRows {
			c.XML(http.StatusOK, channel.CreateFeed())
		} else {
			NewInternalError(err).Abort(c)
		}
		return
	}
	// map from tag.id to translation
	uniqTags := map[int64]string{}
	for _, cu := range item.ContentUnits {
		for _, id := range cu.tagIDs {
			uniqTags[id] = ""
		}
	}
	r := BaseRequest{
		Language: languages[0],
	}
	errH := handleTagsTranslation(db, r, uniqTags)
	if errH != nil {
		errH.Abort(c)
		return
	}
	var nameToIgnore = regexp.MustCompile("kitei-makor|lelo-mikud")
	var lastPubDate time.Time
	for _, cu := range item.ContentUnits {
		files := cu.Files
		tags := make([]string, len(cu.tagIDs))
		for i, id := range cu.tagIDs {
			tags[i] = uniqTags[id]
		}
		for _, file := range files {
			if nameToIgnore.MatchString(file.Name) {
				continue
			}

			// TODO: change title and description
			url := fmt.Sprintf("%s%s%s", consts.CDN, file.ID, filepath.Ext(file.Name))
			description := cu.Description
			if description == "" {
				description = cu.Name
			}
			channel.Items = append(channel.Items, &podcastItem{
				Title:       cu.Name,
				Link:        url,
				PubDate:     file.CreatedAt.Format(time.RFC822),
				Description: description,
				Enclosure: &podcastEnclosure{
					Url:    url,
					Length: file.Size,
					Type:   consts.MEDIA_MP3,
				},
				Guid:     url,
				Duration: convertDuration(cu.Duration),
				Summary:  description,
				Image:    &itunesImage{Href: href},
				Keywords: strings.Join(tags, ","),
				Explicit: "no",
			})
			if file.CreatedAt.After(lastPubDate) {
				lastPubDate = file.CreatedAt
			}
		}
	}
	channel.LastBuildDate = lastPubDate.Format(time.RFC1123)
	tags := make([]string, len(uniqTags))
	i := 0
	for _, k := range uniqTags {
		tags[i] = k
		i++
	}
	channel.Keywords = strings.Join(tags, ",")
	feed := channel.CreateFeed()
	feedXml, err := xml.Marshal(feed)
	payload := []byte(xml.Header + string(feedXml))
	c.Header("Content-Length", fmt.Sprintf("%d", len(payload)))
	c.Header("ETag", etag("feed", payload))
	c.Header("Last-Modified", channel.LastBuildDate)
	c.Data(http.StatusOK, "application/xml; charset=utf-8", payload)
}

func etag(name string, data []byte) string {
	crc := crc32.ChecksumIEEE(data)
	return fmt.Sprintf(`W/"%s-%d-%08X"`, name, len(data), crc)
}