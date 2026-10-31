import { test, describe } from "node:test";
import assert from "node:assert/strict";
import crypto from "node:crypto";

// RFC 6962 Merkle Tree leaf hashing: SHA-256(0x00 || data)
function hashLeaf(leafData: string): Buffer {
  const buf = Buffer.from(leafData, "utf8");
  const prefix = Buffer.from([0x00]);
  return crypto.createHash("sha256").update(Buffer.concat([prefix, buf])).digest();
}

// RFC 6962 Merkle Tree node hashing: SHA-256(0x01 || left || right)
function hashInternalNode(left: Buffer, right: Buffer): Buffer {
  const prefix = Buffer.from([0x01]);
  return crypto.createHash("sha256").update(Buffer.concat([prefix, left, right])).digest();
}

// Compute root from leaves
function computeMerkleRoot(leaves: string[]): string {
  if (leaves.length === 0) return "";
  let nodes = leaves.map(hashLeaf);

  while (nodes.length > 1) {
    const nextLevel: Buffer[] = [];
    for (let i = 0; i < nodes.length; i += 2) {
      if (i + 1 < nodes.length) {
        nextLevel.push(hashInternalNode(nodes[i], nodes[i + 1]));
      } else {
        // Odd leaf is carried forward
        nextLevel.push(nodes[i]);
      }
    }
    nodes = nextLevel;
  }
  return nodes[0].toString("hex");
}

// Verify inclusion proof
function verifyProof(leaf: string, proof: { position: "left" | "right"; hash: string }[], rootHex: string): boolean {
  let current = hashLeaf(leaf);
  for (const step of proof) {
    const sibling = Buffer.from(step.hash, "hex");
    if (step.position === "left") {
      current = hashInternalNode(sibling, current);
    } else {
      current = hashInternalNode(current, sibling);
    }
  }
  return current.toString("hex").toLowerCase() === rootHex.toLowerCase();
}

describe("RFC 6962 Cryptographic Merkle Tree Audit", () => {
  test("Computes deterministic root for 2 leaves", () => {
    const leafA = "ORDER-001:amount=100000";
    const leafB = "ORDER-002:amount=200000";
    const root = computeMerkleRoot([leafA, leafB]);

    assert.equal(typeof root, "string");
    assert.equal(root.length, 64); // SHA-256 hex is 64 characters
  });

  test("Verifies valid inclusion proof for leaf in 2-leaf tree", () => {
    const leafA = "ORDER-001:amount=100000";
    const leafB = "ORDER-002:amount=200000";
    const rootHex = computeMerkleRoot([leafA, leafB]);

    const leafBHash = hashLeaf(leafB).toString("hex");
    const proofForA: { position: "left" | "right"; hash: string }[] = [
      { position: "right", hash: leafBHash },
    ];

    assert.equal(verifyProof(leafA, proofForA, rootHex), true);
  });

  test("Rejects tampered leaf data against authentic root", () => {
    const leafA = "ORDER-001:amount=100000";
    const leafB = "ORDER-002:amount=200000";
    const rootHex = computeMerkleRoot([leafA, leafB]);

    const leafBHash = hashLeaf(leafB).toString("hex");
    const proof: { position: "left" | "right"; hash: string }[] = [
      { position: "right", hash: leafBHash },
    ];

    const tamperedLeaf = "ORDER-001:amount=999999";
    assert.equal(verifyProof(tamperedLeaf, proof, rootHex), false);
  });
});

describe("Double-Entry Balance Invariants", () => {
  test("Invariant 1: Debits == Credits (Global Zero-Sum)", () => {
    const entries = [
      { account: "collector:ondc_settlement", direction: "debit", amountPaise: 100000 },
      { account: "receiver:ondc_settlement", direction: "credit", amountPaise: 95000 },
      { account: "tax:tds_payable", direction: "credit", amountPaise: 100 },
      { account: "tax:gst_tcs_payable", direction: "credit", amountPaise: 500 },
      { account: "sa:tolerance_adjustment", direction: "credit", amountPaise: 4400 },
    ];

    let totalDebits = 0;
    let totalCredits = 0;
    for (const e of entries) {
      if (e.direction === "debit") totalDebits += e.amountPaise;
      else totalCredits += e.amountPaise;
    }

    assert.equal(totalDebits, totalCredits);
    assert.equal(totalDebits - totalCredits, 0);
  });
});
