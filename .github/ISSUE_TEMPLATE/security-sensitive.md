---
name: Security-sensitive report guidance
about: Read before reporting a vulnerability or sensitive exposure
labels: security
---

# Do not submit sensitive details in a public issue

Stop if the report contains a secret, credential, token, private key, unredacted
KYC/payment record, exploitable reproduction, production identifier, or other
material that would increase risk if public.

Use the repository owner's approved private security-reporting channel. If the
repository has not configured one, privately request a channel from the owner
without including the sensitive content. Do not paste the content into a public
issue, pull request, build log, task report, or chat transcript.

A public-safe coordination issue may state only that private security contact is
required, the affected high-level area, and whether immediate human triage is
needed. It must not contain exploit steps or sensitive evidence.

Codex does not approve risk acceptance, disclosure timing, legal obligations,
credential rotation completion, or production deployment. Those require the
designated human owners and the canonical execution protocol.
