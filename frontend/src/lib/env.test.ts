import { expect, it } from "vitest";
import { parseLiteralEnv } from "../../config/env";

it("reads server credentials literally with the same dotenv behavior as the Go service", () => {
  expect(
    parseLiteralEnv(
      "TOKEN=\"abc$unchanged#literal\"\nexport OTHER='value$token'\n# separate comment\nPLAIN=123#suffix\n",
    ),
  ).toEqual({
    TOKEN: "abc$unchanged#literal",
    OTHER: "value$token",
    PLAIN: "123#suffix",
  });
});
