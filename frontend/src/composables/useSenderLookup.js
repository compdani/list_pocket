import { computed } from "vue";

function normalizeEmail(value) {
  return String(value || "").trim();
}

function addUniqueEmail(list, value) {
  const email = normalizeEmail(value);
  if (!email || list.includes(email)) {
    return;
  }
  list.push(email);
}

export function useSenderLookup(serverConfigRef, messengerRef) {
  const smtpSenders = computed(() => {
    const serverConfig = serverConfigRef?.value;
    if (!serverConfig || !Array.isArray(serverConfig.smtp_senders)) {
      return [];
    }

    return serverConfig.smtp_senders.map((sender) => ({
      messenger: String(sender.messenger || "").trim(),
      name: String(sender.name || "").trim(),
      fromAddresses: Array.isArray(sender.from_addresses) ? sender.from_addresses : [],
      defaultFromEmail: normalizeEmail(sender.default_from_email),
    }));
  });

  const availableMessengers = computed(() => {
    const serverConfig = serverConfigRef?.value;
    const messengers = serverConfig && Array.isArray(serverConfig.messengers)
      ? serverConfig.messengers.map((item) => String(item || "").trim()).filter(Boolean)
      : [];

    return messengers.length > 0 ? messengers : ["email"];
  });

  const getSMTPMetaForMessenger = (messenger) => (
    smtpSenders.value.find((item) => item.messenger === messenger) || null
  );

  const availableFromAddresses = computed(() => {
    const selectedMessenger = String(messengerRef?.value || "").trim() || "email";
    const exact = getSMTPMetaForMessenger(selectedMessenger);

    if (exact && Array.isArray(exact.fromAddresses) && selectedMessenger !== "email") {
      return exact.fromAddresses.map(normalizeEmail).filter(Boolean);
    }

    if (selectedMessenger === "email") {
      return smtpSenders.value.reduce((accumulator, sender) => {
        if (!Array.isArray(sender.fromAddresses)) {
          return accumulator;
        }
        sender.fromAddresses.forEach((address) => addUniqueEmail(accumulator, address));
        return accumulator;
      }, []);
    }

    return exact && Array.isArray(exact.fromAddresses)
      ? exact.fromAddresses.map(normalizeEmail).filter(Boolean)
      : [];
  });

  const defaultFromEmail = computed(() => {
    const selectedMessenger = String(messengerRef?.value || "").trim() || "email";
    const serverConfig = serverConfigRef?.value || {};
    const smtpMeta = getSMTPMetaForMessenger(selectedMessenger);
    return normalizeEmail(smtpMeta?.defaultFromEmail) || normalizeEmail(serverConfig.from_email);
  });

  return {
    availableFromAddresses,
    availableMessengers,
    defaultFromEmail,
    smtpSenders,
  };
}
