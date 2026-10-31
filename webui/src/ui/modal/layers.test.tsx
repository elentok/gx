import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useIsTopLayer } from "./layers";

afterEach(cleanup);

function Layer({ name, tops }: { name: string; tops: Record<string, () => boolean> }) {
  tops[name] = useIsTopLayer();
  return null;
}

describe("useIsTopLayer", () => {
  it("reports only the last mounted layer as topmost, and the one beneath again after it unmounts", () => {
    const tops: Record<string, () => boolean> = {};
    const { rerender } = render(<Layer name="a" tops={tops} />);
    expect(tops.a()).toBe(true);
    rerender(
      <>
        <Layer name="a" tops={tops} />
        <Layer name="b" tops={tops} />
      </>,
    );
    expect(tops.a()).toBe(false);
    expect(tops.b()).toBe(true);
    rerender(<Layer name="a" tops={tops} />);
    expect(tops.a()).toBe(true);
  });
});
