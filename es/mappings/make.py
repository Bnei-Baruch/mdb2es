#!/usr/bin/python2
# -*- coding: utf-8 -*-

# TODO: Rewrite this into go.
# This file creates all mapping for Elastic. It is required because some
# features are supported for some languages but not others. Also specific
# languages need specific treatment such as transliteration for cyrillic other
# specific tokenization for CJK.

# Mapping generated here requires the following Elasticsearch plugins:
#   https://www.elastic.co/guide/en/elasticsearch/guide/current/hunspell.html
#   To install download: he_IL.aff, he_IL.dic, settings.yml files from
#   https://github.com/elastic/hunspell/tree/master/dicts/he_IL
#   and put under: elasticsearch-6.X.X/config/hunspell/he_IL
#   Additional dictionary terms are managed in .delta.dic files located in search/hunspell directory of the repo.
#   Copy these files into corresponding folders inside elasticsearch-6.X.X/config/hunspell/
#   For Hebrew dictionary files, the supported format is ISO 8859-8.
#
#
# Deprecated plugins (already not in use):
# Hebrew analyzer plugin:
#   https://github.com/synhershko/elasticsearch-analysis-hebrew
# Phonetic plugin:
#   https://www.elastic.co/guide/en/elasticsearch/plugins/current/analysis-phonetic.html
#   sudo bin/elasticsearch-plugin install analysis-phonetic
# ICU plugin (for transliteration):
#   sudo bin/elasticsearch-plugin install analysis-icu

import json
import os

# List of all languages
ENGLISH = "en"
HEBREW = "he"
RUSSIAN = "ru"
SPANISH = "es"
ITALIAN = "it"
GERMAN = "de"
DUTCH = "nl"
FRENCH = "fr"
PORTUGUESE = "pt"
TURKISH = "tr"
POLISH = "pl"
ARABIC = "ar"
HUNGARIAN = "hu"
FINNISH = "fi"
LITHUANIAN = "lt"
JAPANESE = "ja"
BULGARIAN = "bg"
GEORGIAN = "ka"
NORWEGIAN = "no"
SWEDISH = "sv"
CROATIAN = "hr"
CHINESE = "zh"
PERSIAN = "fa"
ROMANIAN = "ro"
HINDI = "hi"
UKRAINIAN = "ua"
MACEDONIAN = "mk"
SLOVENIAN = "sl"
LATVIAN = "lv"
SLOVAK = "sk"
CZECH = "cs"
AMHARIC = "am"

# Lang groups
ALL = 'ALL_LANGS'
CYRILLIC = 'CYRILLIC'
CJK = 'CJK'
LANG_GROUPS = {
    ALL: [
        ENGLISH, HEBREW, RUSSIAN, SPANISH, ITALIAN, GERMAN, DUTCH, FRENCH,
        PORTUGUESE, TURKISH, POLISH, ARABIC, HUNGARIAN, FINNISH, LITHUANIAN,
        JAPANESE, BULGARIAN, GEORGIAN, NORWEGIAN, SWEDISH, CROATIAN, CHINESE,
        PERSIAN, ROMANIAN, HINDI, MACEDONIAN, SLOVENIAN, LATVIAN, SLOVAK,
        CZECH, UKRAINIAN, AMHARIC,
    ],
    CYRILLIC: [RUSSIAN, BULGARIAN, MACEDONIAN, UKRAINIAN],
    CJK: [CHINESE, JAPANESE],
}

# Units indexing
LANGUAGE_ANALYZER = {
    ENGLISH: "english_synonym",
    HEBREW: "hebrew_synonym",
    RUSSIAN: "russian_synonym",
    SPANISH: "spanish_synonym",

    # In order to allow synonyms in other languages,
    # reimplement their analyzer by adding the necessary filters for each language
    # + the synonym_graph filter and defining a new analyzer that include this filters.
    # List of definitions for each language analyzer are available here:
    # https://www.elastic.co/guide/en/elasticsearch/reference/6.7/analysis-lang-analyzer.html#spanish-analyzer

    ITALIAN: "italian",
    GERMAN: "german",
    DUTCH: "dutch",
    FRENCH: "french",
    PORTUGUESE: "portuguese",
    TURKISH: "turkish",
    POLISH: "standard",
    ARABIC: "arabic",
    HUNGARIAN: "hungarian",
    FINNISH: "finnish",
    LITHUANIAN: "lithuanian",
    JAPANESE: "cjk",
    BULGARIAN: "bulgarian",
    GEORGIAN: "standard",
    NORWEGIAN: "norwegian",
    SWEDISH: "swedish",
    CROATIAN: "standard",
    CHINESE: "cjk",
    PERSIAN: "persian",
    ROMANIAN: "romanian",
    HINDI: "hindi",
    UKRAINIAN: "standard",
    MACEDONIAN: "standard",
    SLOVENIAN: "standard",
    LATVIAN: "latvian",
    SLOVAK: "standard",
    CZECH: "czech",
    AMHARIC: "standard",
}

LANGUAGE_ANALYZER_WITHOUT_SYNONYMS = {
    ENGLISH: "english",
    HEBREW: "he",
    RUSSIAN: "russian",
    SPANISH: "spanish",
    ITALIAN: "italian",
    GERMAN: "german",
    DUTCH: "dutch",
    FRENCH: "french",
    PORTUGUESE: "portuguese",
    TURKISH: "turkish",
    POLISH: "standard",
    ARABIC: "arabic",
    HUNGARIAN: "hungarian",
    FINNISH: "finnish",
    LITHUANIAN: "lithuanian",
    JAPANESE: "cjk",
    BULGARIAN: "bulgarian",
    GEORGIAN: "standard",
    NORWEGIAN: "norwegian",
    SWEDISH: "swedish",
    CROATIAN: "standard",
    CHINESE: "cjk",
    PERSIAN: "persian",
    ROMANIAN: "romanian",
    HINDI: "hindi",
    UKRAINIAN: "standard",
    MACEDONIAN: "standard",
    SLOVENIAN: "standard",
    LATVIAN: "latvian",
    SLOVAK: "standard",
    CZECH: "czech",
    AMHARIC: "standard",
}

SYNONYM_GRAPH_FILTER_IMP = {
    "type": "synonym_graph",
    "tokenizer": "keyword",
    "synonyms": [],
}

LANGUAGE_ANALYZER_IMP = {
    ENGLISH: {
        "english_synonym": {
            "tokenizer": "standard",
            "filter": [
                "english_possessive_stemmer",
                "lowercase",
                "english_stop",
                "english_stemmer",
                "synonym_graph",
            ]
        }
    },
    HEBREW: {
        "hebrew_synonym": {
            "tokenizer": "standard",
            "filter": [
                # The order here is important. As hunspell in many cases produces alternative
                # tokens synonym graph is not able to consume non linear (graph) tokens and fails
                # So for now until issue (https://github.com/elastic/elasticsearch/issues/29426)
                # solved we have to apply synonym before hunspell.
                "synonym_graph",
                "he_IL",
            ],
            "char_filter": [
                "quotes"
            ]
        },
        "he": {
            "tokenizer": "standard",
            "filter": [
                "he_IL"
            ],
            "char_filter": [
                "quotes"
            ]
        }
    },
    RUSSIAN: {
        "russian_synonym": {
            "tokenizer": "standard",
            "filter": [
                "lowercase",
                "russian_stop",
                "russian_stemmer",
                "synonym_graph"
            ]
        }
    },
    SPANISH: {
        "spanish_synonym": {
            "tokenizer": "standard",
            "filter": [
                "lowercase",
                "spanish_stop",
                "spanish_stemmer",
                "synonym_graph"
            ]
        },
    }
}

LANGUAGE_FILTERS_IMP = {
    ENGLISH: {
        "english_stop": {
            "type": "stop",
            "stopwords": "_english_"
        },
        "english_stemmer": {
            "type": "stemmer",
            "language": "english"
        },
        "english_possessive_stemmer": {
            "type": "stemmer",
            "language": "possessive_english"
        },
        "synonym_graph": SYNONYM_GRAPH_FILTER_IMP
    },
    HEBREW: {
        "he_IL": {
            "type": "hunspell",
            "locale": "he_IL",
            "dedup": True,
        },
        "synonym_graph": SYNONYM_GRAPH_FILTER_IMP
    },
    RUSSIAN: {
        "russian_stop": {
            "type": "stop",
            "stopwords": "_russian_"
        },
        "russian_stemmer": {
            "type": "stemmer",
            "language": "russian"
        },
        "synonym_graph": SYNONYM_GRAPH_FILTER_IMP
    },
    SPANISH: {
        "spanish_stop": {
            "type": "stop",
            "stopwords": "_spanish_"
        },
        "spanish_stemmer": {
            "type": "stemmer",
            "language": "light_spanish"
        },
        "synonym_graph": SYNONYM_GRAPH_FILTER_IMP
    },
}

# Phonetic analyzer
BEIDER_MORSE_LANGUAGE_SET = {
    CYRILLIC: 'cyrillic',
    ENGLISH: 'english',
    FRENCH: 'french',
    GERMAN: 'german',
    HEBREW: 'hebrew',
    HUNGARIAN: 'hungarian',
    POLISH: 'polish',
    ROMANIAN: 'romanian',
    RUSSIAN: 'russian',
    SPANISH: 'spanish',
}


def beider_morse_language_set(lang):
    if lang in BEIDER_MORSE_LANGUAGE_SET:
        return BEIDER_MORSE_LANGUAGE_SET[lang]
    elif lang in LANG_GROUPS[CYRILLIC]:
        return BEIDER_MORSE_LANGUAGE_SET[CYRILLIC]
    else:
        return None


def is_cyrillic(lang, something):
    return something if lang in LANG_GROUPS[CYRILLIC] else None


def get_analyzer_imp(lang):
    if lang in LANGUAGE_ANALYZER_IMP:
        return LANGUAGE_ANALYZER_IMP[lang]
    else:
        return None


def get_filters_imp(lang):
    if lang in LANGUAGE_FILTERS_IMP:
        return LANGUAGE_FILTERS_IMP[lang]
    else:
        return None


SETTINGS = {
    "index": {
        "number_of_shards": 1,
        "number_of_replicas": 0,
        "analysis": {
            "analyzer": get_analyzer_imp,
            # "analyzer": {
            #      Tested, but didnt bring quality enough results:
            #     "phonetic_analyzer": {
            #       "tokenizer": "standard",
            #       "char_filter": ["quotes"],
            #       "filter": [
            #         "standard",
            #         "lowercase",
            #         lambda lang: is_cyrillic(lang, 'icu_transliterate'),
            #         "custom_phonetic",
            #       ],
            #     },
            # },
            "char_filter": {
                "quotes": {
                    "type": "mapping",
                    "mappings": [
                        "\\u0027\\u0027=>\\u0029",
                        "\\u0091\\u0091=>\\u0029",
                        "\\u0092\\u0092=>\\u0029",
                        "\\u2018\\u2018=>\\u0029",
                        "\\u2019\\u2019=>\\u0029",
                        "\\u201B\\u201B=>\\u0029",
                        "\\u05F3\\u05F3=>\\u0029",
                        "\\u059C\\u059C=>\\u0029",
                        "\\u059D\\u059D=>\\u0029",
                        "\\u0091=>\\u0027",
                        "\\u0092=>\\u0027",
                        "\\u2018=>\\u0027",
                        "\\u2019=>\\u0027",
                        "\\u201B=>\\u0027",
                        "\\u05F3=>\\u0027",
                        "\\u059C=>\\u0027",
                        "\\u059D=>\\u0027",
                        "\\u0022=>",
                        "\\u201C=>",
                        "\\u201D=>",
                        "\\u05F4=>",
                    ],
                },
            },
            "filter": get_filters_imp,
            # "filter": {
            #     "icu_transliterate": lambda lang: is_cyrillic(lang, {
            #       "type": "icu_transform",
            #       "id": "Any-Latin; NFD; [:Nonspacing Mark:] Remove; NFC",
            #     }),
            #     "custom_phonetic": {
            #       "type": "phonetic",
            #       "encoder": "beider_morse",
            #       "replace": True,
            #       "languageset": beider_morse_language_set,
            #     },
            # },
        },
    },
}

RESULTS_TEMPLATE = {
    # "settings": {
    #   "index": {
    #     "number_of_shards": 1,
    #     "number_of_replicas": 0,
    #   },
    # },
    "settings": SETTINGS,
    "mappings": {
        "result": {
            "dynamic": "strict",
            "properties": {
                # Document type, unit, collection, source, tag.
                "result_type": {
                    "type": "keyword",
                },
                # Document index date.
                "index_date": {
                    "type": "date",
                    "format": "strict_date",
                },
                "mdb_uid": {
                    "type": "keyword",
                },
                # Typed uids, are list of entities (uid and entity type) in MDB that
                # this document depends on. For example: "content_unit:lHDLZWxq",
                # "file:0uzDZVqV". We use this list to reindex the document if one
                # the items in this list changes.
                "typed_uids": {
                    "type": "keyword",
                },
                # List of keywords in format filter:value are required for correct
                # filtering of this document by all different filters. Time is handled
                # by effective_date.
                # For example: content_type:DAILY_LESSON or tag:0db5BBS3
                "filter_values": {
                    "type": "keyword",
                },
                # Title, Description and Content are the typical result fields which
                # should have the same tf/idf across all different retult types such
                # as units, collections, sources, topics and others to follow.
                "title": {
                    "type": "text",
                    "analyzer": "standard",
                    "fields": {
                        "language": {
                            "type": "text",
                            "analyzer": lambda x: LANGUAGE_ANALYZER[x],
                        }
                    }
                },
                "full_title": {
                    "type": "text",
                    "analyzer": "standard",
                    "fields": {
                        "language": {
                            "type": "text",
                            "analyzer": lambda x: LANGUAGE_ANALYZER[x],
                        }
                    }
                },
                "description": {
                    "type": "text",
                    "analyzer": "standard",
                    "fields": {
                        "language": {
                            "type": "text",
                            "analyzer": lambda x: LANGUAGE_ANALYZER[x],
                        },
                    },
                },
                "content": {
                    "type": "text",
                    "analyzer": "standard",
                    "fields": {
                        "language": {
                            "type": "text",
                            "analyzer": lambda x: LANGUAGE_ANALYZER[x],
                        },
                    },
                },

                # Suggest field for autocomplete.
                "title_suggest": {
                    "type": "completion",
                    "analyzer": "standard",
                    "contexts": [
                        {
                            "name": "result_type",
                            "type": "category",
                            "path": "result_type",
                        },
                    ],
                    "fields": {
                        "language": {
                            "type": "completion",
                            "analyzer": lambda x: LANGUAGE_ANALYZER[x],
                            "contexts": [
                                {
                                    "name": "result_type",
                                    "type": "category",
                                    "path": "result_type",
                                },
                            ],
                        }
                    }
                },

                # Content unit specific fields.
                "effective_date": {
                    "type": "date",
                    "format": "strict_date",
                },
            }
        }
    }
}

SEARCH_LOGS_TEMPLATE = {
    "mappings": {
        "search_logs": {
            # We use dynamic mappinng only for search logs.
            "dynamic_templates": [
                {
                    "strings_as_keywords": {
                        "match_mapping_type": "string",
                        "mapping": {
                            "type": "keyword",
                        },
                    },
                },
            ],
            "dynamic": "strict",
            "properties": {
                # Search log key, search_id and timestamp.
                "search_id": {
                    "type": "keyword",
                },
                "created": {
                    "type": "date",
                },

                # Search log type, i.e., "query" or "click".
                "log_type": {
                    "type": "keyword",
                },

                # Query log type fields.
                "query": {
                    "type": "object",
                    "enabled": True,
                    "dynamic": "strict",
                    "properties": {
                        "term": {
                            "type": "keyword",
                        },
                        "exact_terms": {
                            "type": "keyword",
                        },
                        "original": {
                            "type": "keyword",
                        },
                        "filters": {
                            "type": "object",
                            "enabled": True,
                            "dynamic": True,
                        },
                        "language_order": {
                            "type": "keyword",
                        },
                        "deb": {
                            "type": "boolean",
                        },
                        "intents": {
                            "type": "object",
                            "enabled": False,
                            "dynamic": "strict",
                        },
                    },
                },
                "from": {
                    "type": "integer",
                },
                "size": {
                    "type": "integer",
                },
                "suggestion": {
                    "type": "keyword",
                },
                "sort_by": {
                    "type": "keyword",
                },
                "query_result": {
                    "type": "object",
                    "enabled": False,
                    "dynamic": "strict",
                },
                "error": {
                    "type": "object",
                    "enabled": False,
                    "dynamic": "strict",
                },

                # Click log type fields.
                "mdb_uid": {
                    "type": "keyword",
                },
                "index": {
                    "type": "keyword",
                },
                "result_type": {
                    "type": "keyword",
                },
                "rank": {
                    "type": "integer",
                },

                # Log execution time for search components.
                "execution_time_log": {
                    "type": "nested",
                    "properties": {
                        "operation": {"type": "keyword"},
                        "time": {"type": "integer"},
                    },
                },
            },
        },
    },
}

GRAMMARS_TEMPLATE = {
    "settings": SETTINGS,
    "mappings": {
        "grammars": {
            "dynamic": "strict",
            "properties": {
                # Percolator query for matching rules with $Text variable
                "query": {
                    "type": "percolator"
                },
                # Text query from user. Assigned only in query time. Must be defined in index for percolator functionality.
                "search_text": {
                    "type": "text",
                    "analyzer": lambda x: LANGUAGE_ANALYZER_WITHOUT_SYNONYMS[x]
                },
                "grammar_rule": {
                    "dynamic": "strict",
                    "properties": {
                        # Hit type, e.g., "landing-page" or other grammars later on..
                        "hit_type": {
                            "type": "keyword",
                        },
                        # Intent, e.g., which landing page, "conventions" or "lessons".
                        "intent": {
                            "type": "keyword",
                        },
                        # Set of variables for a grammar rule, e.g., [congress $Year $ConventionLocation]
                        # will lead to two keywords: ['$Year', '$ConventionLocation']
                        "variables": {
                            "type": "keyword",
                        },
                        # One value for each variable, e.g., "2019" for $Year or "Bulgaria" for $ConventionLocation.
                        "values": {
                            "type": "keyword",
                        },
                        # Grammar rule: [congress $Year $ConventionLocation] or [$Year $ConventionLocation congress]
                        "rules": {
                            "type": "text",
                            "analyzer": "standard",
                            "fields": {
                                "language": {
                                    "type": "text",
                                    "analyzer": lambda x: LANGUAGE_ANALYZER[x],
                                }
                            }
                        },
                        # Suggest field for autocomplete.
                        "rules_suggest": {
                            "type": "completion",
                            "analyzer": "standard",
                            "fields": {
                                "language": {
                                    "type": "completion",
                                    "analyzer": lambda x: LANGUAGE_ANALYZER[x],
                                }
                            }
                        },
                    }
                }
            }
        }
    }
}


def resolve(lang, value):
    if isinstance(value, dict):
        vals = [(k, resolve(lang, v)) for (k, v) in value.iteritems()]
        return dict([(k, v) for k, v in vals if v is not None])
    elif isinstance(value, list):
        return [x for x in [resolve(lang, v) for v in value] if x is not None]
    elif callable(value):
        return value(lang)
    else:
        return value


if __name__ == "__main__":
    for language in LANG_GROUPS[ALL]:
        with open(os.path.join('..', '..', 'data', 'es', 'mappings', 'results', 'results-%s.json' % language),
                  'w') as f:
            json.dump(resolve(language, RESULTS_TEMPLATE), f, indent=4)
        with open(os.path.join('..', '..', 'data', 'es', 'mappings', 'grammars', 'grammars-%s.json' % language),
                  'w') as f:
            json.dump(resolve(language, GRAMMARS_TEMPLATE), f, indent=4)
    # Without languages
    with open(os.path.join('..', '..', 'data', 'es', 'mappings', 'search_logs.json'), 'w') as f:
        json.dump(resolve('xx', SEARCH_LOGS_TEMPLATE), f, indent=4)
