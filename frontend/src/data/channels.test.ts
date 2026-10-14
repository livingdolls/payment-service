import { describe, expect, it } from "vitest";
import catalog from "../../../testdata/xendit-idr-channels.json";
import { buildProperties, channelByCode, channels } from "./channels";

const fields = {
  name: "Ayu Prameswari",
  phone: "+628123456789",
  cashtag: "$ayu",
  returnUrl: "https://store.example/paid",
  advanced: "{}",
  advancedEnabled: false,
};

describe("API channel compatibility", () => {
  it("matches every code and eligible profile from the backend channel catalog", () => {
    const actual = channels
      .map((channel) => [channel.code, channel.available])
      .sort();
    const expected = catalog.channels
      .map((channel) => [channel.channel_code, channel.eligible])
      .sort();
    expect(actual).toEqual(expected);
  });

  it("maps provider-specific properties for VA, retail, wallet, and QRIS", () => {
    expect(
      buildProperties(channelByCode("BCA_VIRTUAL_ACCOUNT")!, fields),
    ).toEqual({ display_name: "Ayu Prameswari" });
    expect(buildProperties(channelByCode("ALFAMART")!, fields)).toEqual({
      payer_name: "Ayu Prameswari",
    });
    expect(buildProperties(channelByCode("OVO")!, fields)).toEqual({
      account_mobile_number: "+628123456789",
    });
    expect(buildProperties(channelByCode("JENIUSPAY")!, fields)).toEqual({
      cashtag: "$ayu",
    });
    expect(buildProperties(channelByCode("QRIS")!, fields)).toEqual({});
    expect(buildProperties(channelByCode("GOPAY")!, fields)).toEqual({
      success_return_url: fields.returnUrl,
      failure_return_url: fields.returnUrl,
      cancel_return_url: fields.returnUrl,
    });
  });

  it("rejects invalid advanced JSON, empty card properties, and insecure return URLs", () => {
    expect(() =>
      buildProperties(channelByCode("QRIS")!, {
        ...fields,
        advancedEnabled: true,
        advanced: "[]",
      }),
    ).toThrow("objek JSON");
    expect(() => buildProperties(channelByCode("CARDS")!, fields)).toThrow(
      "Isi properti",
    );
    expect(() =>
      buildProperties(channelByCode("DANA")!, {
        ...fields,
        returnUrl: "http://store.example",
      }),
    ).toThrow("HTTPS");
    expect(
      buildProperties(channelByCode("QRIS")!, {
        ...fields,
        advancedEnabled: true,
        advanced: '{"expires_at":"2026-10-20T00:00:00Z"}',
      }),
    ).toEqual({ expires_at: "2026-10-20T00:00:00Z" });
  });
});
