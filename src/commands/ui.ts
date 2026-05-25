import * as clack from "@clack/prompts";

export interface SelectOption {
  value: string;
  label: string;
  hint?: string;
}

export interface MultiSelectOption extends SelectOption {
  required?: boolean;
}

export interface CommandUi {
  intro(message: string): void;
  outro(message: string): void;
  info(message: string): void;
  note(message: string, title?: string): void;
  spinner(): {
    start(message: string): void;
    stop(message: string): void;
  };
  confirm(options: { message: string; initialValue?: boolean }): Promise<boolean>;
  select(options: {
    message: string;
    options: SelectOption[];
    initialValue?: string;
  }): Promise<string>;
  multiselect(options: {
    message: string;
    options: MultiSelectOption[];
    initialValues?: string[];
  }): Promise<string[]>;
}

export const clackUi: CommandUi = {
  intro(message) {
    clack.intro(message);
  },
  outro(message) {
    clack.outro(message);
  },
  info(message) {
    clack.log.info(message);
  },
  note(message, title) {
    clack.note(message, title);
  },
  spinner() {
    return clack.spinner();
  },
  async confirm({ message, initialValue }) {
    const result = await clack.confirm({ message, initialValue });
    if (clack.isCancel(result)) {
      throw new Error("Command cancelled");
    }
    return result;
  },
  async select({ message, options, initialValue }) {
    const result = await clack.select({
      message,
      initialValue,
      options,
    });
    if (clack.isCancel(result)) {
      throw new Error("Command cancelled");
    }
    return result;
  },
  async multiselect({ message, options, initialValues }) {
    const result = await clack.multiselect({
      message,
      initialValues,
      options: options.map((option) => ({
        value: option.value,
        label: option.label,
        hint: option.hint,
        required: option.required,
      })),
    });
    if (clack.isCancel(result)) {
      throw new Error("Command cancelled");
    }
    return result;
  },
};
