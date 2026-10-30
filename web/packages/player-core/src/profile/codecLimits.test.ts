import { describe, expect, it } from "vitest";

import { commonVariants, deriveCodecLimits, type PlayingCodecs } from "./codecLimits";
import type { CodecLimit } from "./clientProfile";
import type { CodecVariant } from "./probeTables";

const main8: CodecVariant = { codecString: "c.main", bitDepth: 8, profile: "Main" };
const main10: CodecVariant = { codecString: "c.main10", bitDepth: 10, profile: "Main 10" };
const depth8: CodecVariant = { codecString: "d.8", bitDepth: 8 };
const depth10: CodecVariant = { codecString: "d.10", bitDepth: 10 };
const plain: CodecVariant = { codecString: "plain" };
const profileOnly: CodecVariant = { codecString: "p", profile: "Simple" };

function context(entries: [string, CodecVariant[]][]): PlayingCodecs {
  return new Map(entries);
}

describe("commonVariants", () => {
  it("intersects the variants over the contexts where a codec plays, in codec order", () => {
    const progressive = context([
      ["deep", [depth8, depth10]],
      ["profiled", [main8, main10]],
    ]);
    const mediaSource = context([["profiled", [main8]]]);

    expect(
      commonVariants(["profiled", "deep", "silent"], [progressive, mediaSource, context([])]),
    ).toEqual(
      new Map([
        ["profiled", [main8]],
        ["deep", [depth8, depth10]],
      ]),
    );
  });

  it("returns an empty list for a codec whose contexts share no variant", () => {
    expect(
      commonVariants(["deep"], [context([["deep", [depth8]]]), context([["deep", [depth10]]])]),
    ).toEqual(new Map([["deep", []]]));
  });
});

interface LimitCase {
  readonly case: string;
  readonly common: [string, CodecVariant[]][];
  readonly expected: CodecLimit[];
}

const limitCases: LimitCase[] = [
  { case: "nothing plays", common: [], expected: [] },
  {
    case: "every variant plays",
    common: [
      ["profiled", [main8, main10]],
      ["deep", [depth8, depth10]],
      ["plain", [plain]],
    ],
    expected: [
      { codec: "profiled", maxBitDepth: 10, profiles: ["Main", "Main 10"] },
      { codec: "deep", maxBitDepth: 10 },
    ],
  },
  {
    case: "only 8-bit variants play",
    common: [
      ["profiled", [main8]],
      ["deep", [depth8]],
    ],
    expected: [
      { codec: "profiled", maxBitDepth: 8, profiles: ["Main"] },
      { codec: "deep", maxBitDepth: 8 },
    ],
  },
  {
    case: "only the 10-bit variant plays",
    common: [["deep", [depth10]]],
    expected: [{ codec: "deep", maxBitDepth: 10 }],
  },
  {
    case: "a codec has profile names but no bit depths",
    common: [["mpeg4", [profileOnly]]],
    expected: [{ codec: "mpeg4", profiles: ["Simple"] }],
  },
  {
    case: "only a codec without bit depths or profiles plays",
    common: [["plain", [plain]]],
    expected: [],
  },
  { case: "the contexts share no variant", common: [["deep", []]], expected: [] },
];

describe("deriveCodecLimits", () => {
  it.each(limitCases)("derives the limits when $case", ({ common, expected }) => {
    expect(deriveCodecLimits(new Map(common))).toEqual(expected);
  });
});
