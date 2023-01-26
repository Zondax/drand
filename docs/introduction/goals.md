---
title: "Goals"
sidebar_position: 3
---


### Public Randomness
Generating public randomness is the primary functionality of drand. Public
randomness is generated collectively by drand nodes and publicly available. The
main challenge in generating good randomness is that no party involved in the
randomness generation process should be able to predict or bias the final
output. Additionally, the final result has to be third-party verifiable to make
it actually useful for applications like lotteries, sharding, or parameter
generation in security protocols.

A drand randomness beacon is composed of a distributed set of nodes and has two
phases:

- **Setup:** Each node first generates a *long-term public/private key pair*.
  Then all of the public keys are written to a *group file* together with some
  further metadata required to operate the beacon. After this group file has
  been distributed, the nodes perform a *distributed key generation* (DKG)
  protocol to create the collective public key and one private key share per
  server. The participants NEVER see/use the actual (distributed) private key
  explicitly but instead utilize their respective private key shares for the
  generation of public randomness.
- **Generation:** After the setup, the nodes switch to the randomness
  generation mode. Any of the nodes can initiate a randomness generation round
  by broadcasting a message which all the other participants sign using a
  t-of-n threshold version of the *Boneh-Lynn-Shacham* (BLS) signature scheme
  and their respective private key shares. Once any node (or third-party
  observer) has gathered t partial signatures, it can reconstruct the full BLS
  signature (using Lagrange interpolation). The signature is then hashed using
  SHA-256 to ensure that there is no bias in the byte representation of the
  final output. This hash corresponds to the collective random value and can be
  verified against the collective public key.

### Private Randomness
Private randomness generation is the secondary functionality of drand. Clients
can request private randomness from some or all of the drand nodes which
extract it locally from their entropy pools and send it back in encrypted form.
This can be useful to gather randomness from different entropy sources, for
example in embedded devices.

In this mode we assume that a client has a private/public key pair and
encapsulates its public key towards the server's public key using the ECIES
encryption scheme. After receiving a request, the drand node produces 32 random
bytes locally (using Go's `crypto/rand` interface), encrypts them using the
received public key and sends it back to the client.

**Note:** Assuming that clients without good local entropy sources (such as
embedded devices) use this process to gather high entropy randomness to
bootstrap their local PRNGs, we emphasize that the initial client key pair has
to be provided by a trusted source (such as the device manufacturer). Otherwise
we run into the chicken-and-egg problem of how to produce on the client's side
a secure ephemeral key pair for ECIES encryption without a good (local) source
of randomness.
