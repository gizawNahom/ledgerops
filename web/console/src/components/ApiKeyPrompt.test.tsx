// ApiKeyPrompt -- the operator pastes a key once per browser (Pre-requisite
// D10, ADR-010 client-side key delivery). Layer: component.
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiKeyPrompt } from "./ApiKeyPrompt";

describe("ApiKeyPrompt -- the operator pastes a key once per browser", () => {
  it.skip("submitting a pasted key hands it to the console to store and retry", async () => {
    const onSubmit = vi.fn();
    render(<ApiKeyPrompt rejected={false} onSubmit={onSubmit} />);

    await userEvent.type(screen.getByLabelText(/operator api key/i), "a-pasted-key");
    await userEvent.click(screen.getByRole("button", { name: /submit|save|continue/i }));

    expect(onSubmit).toHaveBeenCalledWith("a-pasted-key");
  });

  it.skip("the pasted key is never shoulder-surfable -- masked input, not plain text", () => {
    render(<ApiKeyPrompt rejected={false} onSubmit={vi.fn()} />);
    expect(screen.getByLabelText(/operator api key/i)).toHaveAttribute("type", "password");
  });

  it.skip("@error a rejected key re-shows the form with an inline explanation, not the blank first-load prompt", () => {
    render(<ApiKeyPrompt rejected={true} onSubmit={vi.fn()} />);
    expect(screen.getByText(/key rejected/i)).toBeInTheDocument();
  });
});
