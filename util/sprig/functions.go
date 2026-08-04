package sprig

import (
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

const (
	loopExecutionLimit = 10_000  // Limit the number of loop executions to prevent execution from taking too long
	stringLengthLimit  = 100_000 // Limit the length of strings to prevent memory issues
	sliceSizeLimit     = 10_000  // Limit the size of slices to prevent memory issues
	indentSpacesLimit  = 100     // Limit indentation width to prevent memory issues; indent allocates spaces*lines bytes
)

// TxtFuncMap produces the function map.
//
// Use this to pass the functions into the template engine:
//
//	tpl := template.New("foo").Funcs(sprig.FuncMap()))
//
// TxtFuncMap returns the function map as a plain map[string]any, assignable to any text/template or
// html/template FuncMap (including ntfy's vendored internal/template).
func TxtFuncMap() map[string]any {
	return map[string]any{
		// Date functions
		"ago":            dateAgo,
		"date":           date,
		"dateInZone":     dateInZone,
		"dateModify":     dateModify,
		"duration":       duration,
		"durationRound":  durationRound,
		"htmlDate":       htmlDate,
		"htmlDateInZone": htmlDateInZone,
		"mustDateModify": mustDateModify,
		"mustToDate":     mustToDate,
		"now":            time.Now,
		"toDate":         toDate,
		"unixEpoch":      unixEpoch,

		// Strings
		"trunc":      trunc,
		"trim":       strings.TrimSpace,
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"title":      title,
		"substr":     substring,
		"repeat":     repeat,
		"trimAll":    trimAll,
		"trimPrefix": trimPrefix,
		"trimSuffix": trimSuffix,
		"contains":   contains,
		"hasPrefix":  hasPrefix,
		"hasSuffix":  hasSuffix,
		"quote":      quote,
		"squote":     squote,
		"cat":        cat,
		"indent":     indent,
		"nindent":    nindent,
		"replace":    replace,
		"plural":     plural,
		"sha1sum":    sha1sum,
		"sha256sum":  sha256sum,
		"sha512sum":  sha512sum,
		"adler32sum": adler32sum,
		"toString":   strval,

		// Wrap Atoi to stop errors.
		"atoi":      atoi,
		"seq":       seq,
		"toDecimal": toDecimal,
		"split":     split,
		"splitList": splitList,
		"splitn":    splitn,
		"toStrings": strslice,

		"until":     until,
		"untilStep": untilStep,

		// Basic arithmetic
		"add1":    add1,
		"add":     add,
		"sub":     sub,
		"div":     div,
		"mod":     mod,
		"mul":     mul,
		"randInt": randInt,
		"biggest": maxAsInt64,
		"max":     maxAsInt64,
		"min":     minAsInt64,
		"maxf":    maxAsFloat64,
		"minf":    minAsFloat64,
		"ceil":    ceil,
		"floor":   floor,
		"round":   round,

		// string slices. Note that we reverse the order b/c that's better
		// for template processing.
		"join":      join,
		"sortAlpha": sortAlpha,

		// Defaults
		"default":          defaultValue,
		"empty":            empty,
		"coalesce":         coalesce,
		"all":              all,
		"any":              anyNonEmpty,
		"compact":          compact,
		"mustCompact":      mustCompact,
		"fromJSON":         fromJSON,
		"toJSON":           toJSON,
		"toPrettyJSON":     toPrettyJSON,
		"toRawJSON":        toRawJSON,
		"mustFromJSON":     mustFromJSON,
		"mustToJSON":       mustToJSON,
		"mustToPrettyJSON": mustToPrettyJSON,
		"mustToRawJSON":    mustToRawJSON,
		"ternary":          ternary,

		// Reflection
		"typeOf":     typeOf,
		"typeIs":     typeIs,
		"typeIsLike": typeIsLike,
		"kindOf":     kindOf,
		"kindIs":     kindIs,
		"deepEqual":  reflect.DeepEqual,

		// Paths
		"base":  path.Base,
		"dir":   path.Dir,
		"clean": path.Clean,
		"ext":   path.Ext,
		"isAbs": path.IsAbs,

		// Filepaths
		"osBase":  filepath.Base,
		"osClean": filepath.Clean,
		"osDir":   filepath.Dir,
		"osExt":   filepath.Ext,
		"osIsAbs": filepath.IsAbs,

		// Encoding
		"b64enc": base64encode,
		"b64dec": base64decode,
		"b32enc": base32encode,
		"b32dec": base32decode,

		// Data Structures
		"tuple":  list, // FIXME: with the addition of append/prepend these are no longer immutable.
		"list":   list,
		"dict":   dict,
		"get":    get,
		"set":    set,
		"unset":  unset,
		"hasKey": hasKey,
		"pluck":  pluck,
		"keys":   keys,
		"pick":   pick,
		"omit":   omit,
		"values": values,

		"append":      push,
		"push":        push,
		"mustAppend":  mustPush,
		"mustPush":    mustPush,
		"prepend":     prepend,
		"mustPrepend": mustPrepend,
		"first":       first,
		"mustFirst":   mustFirst,
		"rest":        rest,
		"mustRest":    mustRest,
		"last":        last,
		"mustLast":    mustLast,
		"initial":     initial,
		"mustInitial": mustInitial,
		"reverse":     reverse,
		"mustReverse": mustReverse,
		"uniq":        uniq,
		"mustUniq":    mustUniq,
		"without":     without,
		"mustWithout": mustWithout,
		"has":         has,
		"mustHas":     mustHas,
		"slice":       slice,
		"mustSlice":   mustSlice,
		"concat":      concat,
		"dig":         dig,
		"chunk":       chunk,
		"mustChunk":   mustChunk,

		// Flow Control
		"fail": fail,

		// Regex
		"regexMatch":                 regexMatch,
		"mustRegexMatch":             mustRegexMatch,
		"regexFindAll":               regexFindAll,
		"mustRegexFindAll":           mustRegexFindAll,
		"regexFind":                  regexFind,
		"mustRegexFind":              mustRegexFind,
		"regexReplaceAll":            regexReplaceAll,
		"mustRegexReplaceAll":        mustRegexReplaceAll,
		"regexReplaceAllLiteral":     regexReplaceAllLiteral,
		"mustRegexReplaceAllLiteral": mustRegexReplaceAllLiteral,
		"regexSplit":                 regexSplit,
		"mustRegexSplit":             mustRegexSplit,
		"regexQuoteMeta":             regexQuoteMeta,

		// URLs
		"urlParse": urlParse,
		"urlJoin":  urlJoin,
	}
}
