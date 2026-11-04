import { ApiError } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { fakeLibrary } from "../../../test/libraryFixtures";
import {
  chosenFolder,
  classifyProblem,
  createBody,
  draftOf,
  emptyDraft,
  joinDisplayPath,
  lastPathSegment,
  sameDraft,
  suggestedName,
  updateBody,
} from "./libraryDraft";

const root = { id: "root-1", path: "/media", available: true };
const badRequest = 400;

function problem(code: string, field?: string, libraryId?: string): ApiError {
  return new ApiError({ status: badRequest, title: code, code, field, libraryId });
}

describe("lastPathSegment", () => {
  it.each([
    ["/media/movies", "movies"],
    ["/media/movies/", "movies"],
    ["C:\\Media\\Films", "Films"],
    ["/", ""],
    ["", ""],
  ])("takes the last element of %s", (path, expected) => {
    expect(lastPathSegment(path)).toBe(expected);
  });
});

describe("joinDisplayPath", () => {
  it.each([
    ["/media", "", "/media"],
    ["/media", "Movies/Animation", "/media/Movies/Animation"],
    ["/media/", "Movies", "/media/Movies"],
    ["D:\\Video", "Movies/Kids", "D:\\Video\\Movies\\Kids"],
  ])("joins %s and %s", (rootPath, relative, expected) => {
    expect(joinDisplayPath(rootPath, relative)).toBe(expected);
  });
});

describe("drafts", () => {
  it("suggests the name of the first folder, or the root's when the root is chosen", () => {
    expect(suggestedName([])).toBe("");
    expect(suggestedName([chosenFolder(root, "Movies/Kids", "a")])).toBe("Kids");
    expect(suggestedName([chosenFolder(root, "", "a")])).toBe("media");
  });

  it("builds a create body with new folder refs and trimmed name", () => {
    const draft = {
      ...emptyDraft("shows"),
      name: " Shows ",
      folders: [chosenFolder(root, "Shows", "a")],
    };

    expect(createBody(draft)).toEqual({
      kind: "shows",
      name: "Shows",
      folders: [{ rootId: "root-1", path: "Shows" }],
      metadataLanguage: "",
      options: { scanExtras: true, readNfo: true, localArtwork: true },
    });
  });

  it("keeps folder ids when editing and leaves the kind out", () => {
    const library = fakeLibrary();
    const draft = {
      ...draftOf(library),
      folders: [...draftOf(library).folders, chosenFolder(root, "More", "b")],
    };

    const body = updateBody(draft);

    expect(body.folders).toEqual([
      { folderId: library.folders[0]?.id },
      { rootId: "root-1", path: "More" },
    ]);
    expect("kind" in body).toBe(false);
  });

  it("detects changes", () => {
    const original = draftOf(fakeLibrary());

    expect(sameDraft(original, { ...original, name: " Movies " })).toBe(true);
    expect(sameDraft(original, { ...original, name: "Other" })).toBe(false);
    expect(sameDraft(original, { ...original, language: "de" })).toBe(false);
    expect(sameDraft(original, { ...original, folders: [] })).toBe(false);
    expect(
      sameDraft(original, { ...original, options: { ...original.options, readNfo: false } }),
    ).toBe(false);
  });
});

describe("classifyProblem", () => {
  it.each([
    ["name_invalid", "name"],
    ["conflict", "name"],
    ["language_invalid", "language"],
    ["kind_unsupported", "kind"],
    ["too_many_folders", "folders"],
    ["forbidden", "general"],
    ["internal_error", "general"],
  ])("sorts %s to %s", (code, scope) => {
    expect(classifyProblem(problem(code)).scope).toBe(scope);
  });

  it.each([
    "folder_not_found",
    "folder_unreadable",
    "folder_overlap",
    "folder_reserved",
    "folder_outside_roots",
  ])("sorts %s to the folder named by the field", (code) => {
    expect(classifyProblem(problem(code, "folders[2]", "lib-1"))).toMatchObject({
      scope: "folder",
      folderIndex: 2,
      libraryId: "lib-1",
    });
  });

  it("falls back to the folder list without a usable field", () => {
    expect(classifyProblem(problem("folder_unreadable"))).toMatchObject({ scope: "folders" });
    expect(classifyProblem(problem("folder_unreadable", "folders[x]"))).toMatchObject({
      scope: "folders",
    });
  });

  it("treats other errors as general", () => {
    expect(classifyProblem(new Error("boom"))).toMatchObject({ scope: "general", reason: "boom" });
  });
});
