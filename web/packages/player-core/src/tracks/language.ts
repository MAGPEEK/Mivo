// Language matching for track selection, the same rules as the server's default policy
// (ADR 0023 Notes M4-04a, internal/domain/playback/language.go): ISO 639-1, 639-2/B and 639-2/T
// codes of one language compare equal and only the primary subtag counts.

/** ISO 639-2 code for "no language given"; ffprobe and Matroska write it for untagged tracks. */
const undeterminedLanguage = "und";

/** Most languages a preference list may hold (`ClientProfile.preferredAudioLanguages`). */
export const maxPreferredLanguages = 8;

/** Longest language tag the API accepts, in characters. */
export const maxLanguageTagLength = 16;

// What the API accepts as a preferred language: a primary subtag of two or three letters,
// optionally followed by subtags of letters and digits ("pt-BR", "zh_Hant").
const languageTagPattern = /^[A-Za-z]{2,3}(?:[-_][A-Za-z0-9]+)*$/;

// "<ISO 639-1> [<ISO 639-2/B>] <ISO 639-2/T>" entries; the last code of an entry is the normal
// form. Some entries also carry deprecated ISO 639-1 codes (iw, in, ji), and Norwegian Bokmål
// and Nynorsk share the "nor" form, because files tag Norwegian as any of them. Kept identical
// to the server's table; languages missing here still match when both sides use the same code.
const languageCodes =
  "af afr;sq alb sqi;am amh;ar ara;hy arm hye;az aze;eu baq eus;be bel;" +
  "bn ben;bs bos;br bre;bg bul;my bur mya;ca cat;zh chi zho;hr hrv;cs cze ces;da dan;" +
  "nl dut nld;en eng;eo epo;et est;fo fao;fi fin;fr fre fra;gl glg;ka geo kat;de ger deu;" +
  "el gre ell;gu guj;ha hau;he iw heb;hi hin;hu hun;is ice isl;ig ibo;id in ind;ga gle;gd gla;" +
  "it ita;ja jpn;kn kan;kk kaz;km khm;ko kor;ku kur;ky kir;lo lao;la lat;lv lav;lt lit;" +
  "lb ltz;mk mac mkd;mg mlg;ms may msa;ml mal;mt mlt;mi mao mri;mr mar;mn mon;ne nep;" +
  "nb nn no nob nno nor;oc oci;ps pus;fa per fas;pl pol;pt por;pa pan;ro rum ron;ru rus;" +
  "sr srp;si sin;sk slo slk;sl slv;so som;es spa;sw swa;sv swe;tl tgl;tg tgk;ta tam;" +
  "te tel;th tha;bo tib bod;tr tur;tk tuk;uk ukr;ur urd;uz uzb;vi vie;cy wel cym;xh xho;" +
  "yi ji yid;yo yor;zu zul";

const normalForms: ReadonlyMap<string, string> = new Map(
  languageCodes.split(";").flatMap((entry) => {
    const codes = entry.split(" ");
    const normal = codes.at(-1) ?? entry;
    return codes.map((code) => [code, normal] as const);
  }),
);

/**
 * The ISO 639-2/T form of a language tag, so `de`, `ger`, `deu` and `de-AT` compare equal.
 * Empty, missing and `und` return `""`; unknown codes return their lower-case primary subtag.
 */
export function normalizeLanguage(tag: string | undefined): string {
  const [primary = ""] = (tag ?? "").trim().toLowerCase().replaceAll("_", "-").split("-");
  if (primary === "" || primary === undeterminedLanguage) {
    return "";
  }
  return normalForms.get(primary) ?? primary;
}

/** Whether both tags name the same, determined language. */
export function sameLanguage(first: string | undefined, second: string | undefined): boolean {
  const normal = normalizeLanguage(first);
  return normal !== "" && normal === normalizeLanguage(second);
}

/**
 * Position of the language in the preferred list, or the list length when it is not listed or
 * undetermined. Undetermined entries of the list never match.
 */
export function languageRank(language: string | undefined, preferred: readonly string[]): number {
  const position = preferred.findIndex((candidate) => sameLanguage(candidate, language));
  return position === -1 ? preferred.length : position;
}

/** Whether the API accepts the tag as a preferred language. */
export function isLanguageTag(tag: string): boolean {
  return tag.length <= maxLanguageTagLength && languageTagPattern.test(tag);
}

/**
 * A language list the API accepts, e.g. from `navigator.languages`: tags are trimmed, invalid
 * ones dropped, later tags of a language already listed removed (`de-AT` after `de`), and the
 * rest cut to {@link maxPreferredLanguages}. The order, most wanted first, is kept.
 */
export function preferredLanguageList(tags: readonly string[]): string[] {
  const seen = new Set<string>();
  const list: string[] = [];
  for (const tag of tags.map((candidate) => candidate.trim())) {
    const normal = normalizeLanguage(tag);
    if (isLanguageTag(tag) && normal !== "" && !seen.has(normal)) {
      seen.add(normal);
      list.push(tag);
    }
  }
  return list.slice(0, maxPreferredLanguages);
}
