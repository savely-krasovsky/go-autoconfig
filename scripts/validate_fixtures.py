#!/usr/bin/env python3
"""Validate golden responses without external dependencies."""

import plistlib
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
TESTDATA = ROOT / "handlers" / "testdata"


def fail(message):
    print(f"fixture validation failed: {message}", file=sys.stderr)
    sys.exit(1)


def local(tag):
    return tag.rsplit("}", 1)[-1]


def children(element, name):
    return [child for child in element if local(child.tag) == name]


def values(element):
    return {local(child.tag): (child.text or "") for child in element}


def check_outlook():
    root = ET.parse(TESTDATA / "outlook.xml").getroot()
    if local(root.tag) != "Autodiscover":
        fail("outlook: root element is not Autodiscover")

    users = [element for element in root.iter() if local(element.tag) == "User"]
    if len(users) != 1:
        fail("outlook: expected exactly one User")
    user = values(users[0])
    if user.get("DisplayName") != "user@example.com":
        fail("outlook: unexpected DisplayName")
    if user.get("AutoDiscoverSMTPAddress") != "user@example.com":
        fail("outlook: AutoDiscoverSMTPAddress is missing")

    accounts = [element for element in root.iter() if local(element.tag) == "Account"]
    if len(accounts) != 2:
        fail("outlook: expected IMAP and SMTP accounts")
    if {values(account).get("AccountType") for account in accounts} != {"email"}:
        fail("outlook: AccountType must be email")

    protocols = [element for element in root.iter() if local(element.tag) == "Protocol"]
    kinds = [values(protocol).get("Type") for protocol in protocols]
    if kinds != ["IMAP", "SMTP"]:
        fail(f"outlook: unexpected protocol order {kinds}")

    for protocol in protocols:
        fields = values(protocol)
        for field in ("Server", "Port", "LoginName", "AuthRequired"):
            if not fields.get(field):
                fail(f"outlook: {fields.get('Type')} has no {field}")
        if not fields.get("SSL") and not fields.get("TLS"):
            fail(f"outlook: {fields.get('Type')} has neither SSL nor TLS")


def check_thunderbird():
    root = ET.parse(TESTDATA / "thunderbird.xml").getroot()
    if local(root.tag) != "clientConfig" or root.attrib.get("version") != "1.1":
        fail("thunderbird: invalid clientConfig root")

    providers = [element for element in root.iter() if local(element.tag) == "emailProvider"]
    if len(providers) != 1 or providers[0].attrib.get("id") != "example.com":
        fail("thunderbird: invalid emailProvider")

    for server_type in ("incomingServer", "outgoingServer"):
        servers = children(providers[0], server_type)
        if len(servers) != 1:
            fail(f"thunderbird: expected one {server_type}")
        fields = values(servers[0])
        for field in ("hostname", "port", "socketType", "authentication", "username"):
            if not fields.get(field):
                fail(f"thunderbird: {server_type} has no {field}")
        if fields["socketType"] not in {"SSL", "STARTTLS", "plain"}:
            fail(f"thunderbird: invalid socketType {fields['socketType']}")


def check_apple():
    with open(TESTDATA / "apple.mobileconfig", "rb") as handle:
        profile = plistlib.load(handle)

    if profile.get("PayloadType") != "Configuration":
        fail("apple: PayloadType must be Configuration")
    content = profile.get("PayloadContent")
    if not isinstance(content, list) or len(content) != 1:
        fail("apple: PayloadContent must contain one payload")

    account = content[0]
    if account.get("PayloadType") != "com.apple.mail.managed":
        fail("apple: invalid PayloadType")
    for key in (
        "EmailAccountType",
        "EmailAddress",
        "IncomingMailServerHostName",
        "IncomingMailServerPortNumber",
        "IncomingMailServerUsername",
        "OutgoingMailServerHostName",
        "OutgoingMailServerPortNumber",
        "OutgoingMailServerUsername",
        "PayloadUUID",
        "PayloadIdentifier",
    ):
        if key not in account:
            fail(f"apple: missing {key}")
    if account["EmailAccountType"] != "EmailTypeIMAP":
        fail("apple: EmailAccountType must be EmailTypeIMAP")
    if "HasRemovalPasscode" in profile:
        fail("apple: HasRemovalPasscode is not part of the schema")
    for key in ("SMIMEEncryptionEnabled", "SMIMESigningEnabled", "allowMailDrop"):
        if account.get(key) is not False:
            fail(f"apple: {key} must be false")


def main():
    check_outlook()
    check_thunderbird()
    check_apple()
    print("fixtures ok")


if __name__ == "__main__":
    main()
