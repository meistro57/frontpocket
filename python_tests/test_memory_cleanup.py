from __future__ import annotations

import unittest
from unittest.mock import patch

from frontpocket import memory_cleanup


class MemoryCleanupTests(unittest.TestCase):
    def _point(self, payload=None, vector=None):
        return {
            "id": "pt-1",
            "payload": payload
            or {
                "memory_id": "mem-1",
                "source_title": "Modular Steel Inventory System",
                "source_quote": "This is a complete quote with enough detail.",
                "text": "This is a complete quote with enough detail and context for reflection.",
                "speaker": "user",
                "timestamp": "2026-06-26T12:00:00Z",
                "conversation_id": "conv-1",
                "project": "FrontPocket",
            },
            "vector": vector,
        }

    def test_valid_record_clean(self):
        result = memory_cleanup.clean_point(
            self._point(),
            seen_ids=set(),
            seen_hashes={},
            include_vectors=False,
            repair_quotes=False,
        )
        self.assertEqual(result.payload["cleanup_status"], "clean")
        self.assertTrue(result.payload["safe_for_reflection"])

    def test_empty_project_normalized_and_hint(self):
        point = self._point(
            payload={
                "memory_id": "mem-2",
                "source_title": "Modular Steel Inventory System",
                "source_quote": "A complete quote with enough context for review.",
                "text": "A complete quote with enough context for review and cleanup behavior.",
                "speaker": "user",
                "timestamp": "2026-06-26T12:00:00Z",
                "conversation_id": "conv-1",
                "project": "",
            }
        )
        result = memory_cleanup.clean_point(point, seen_ids=set(), seen_hashes={}, include_vectors=False, repair_quotes=False)
        self.assertIsNone(result.payload["project_original"])
        self.assertEqual(result.payload["project_hint"], "Steel Inventory")

    def test_truncated_quote_marked(self):
        point = self._point(
            payload={
                "memory_id": "mem-3",
                "source_title": "Any",
                "source_quote": "...oken fragment still continuing",
                "text": "...oken fragment still continuing",
                "speaker": "user",
                "timestamp": "2026-06-26T12:00:00Z",
                "conversation_id": "conv-1",
                "project": "X",
            }
        )
        result = memory_cleanup.clean_point(point, seen_ids=set(), seen_hashes={}, include_vectors=False, repair_quotes=False)
        self.assertIn(result.payload["quote_quality"], {"truncated", "malformed", "partial"})

    def test_repaired_quote_preserves_original(self):
        point = self._point(
            payload={
                "memory_id": "mem-4",
                "source_title": "Any",
                "source_quote": "This quote ends mid sentence",
                "text": "This quote ends mid sentence and contains context",
                "speaker": "user",
                "timestamp": "2026-06-26T12:00:00Z",
                "conversation_id": "conv-1",
                "project": "X",
            }
        )
        result = memory_cleanup.clean_point(point, seen_ids=set(), seen_hashes={}, include_vectors=False, repair_quotes=True)
        self.assertEqual(result.payload["source_quote_original"], "This quote ends mid sentence")
        self.assertNotEqual(result.payload["quote_repair_method"], "none")

    def test_missing_quote_blocks_reflection(self):
        point = self._point(
            payload={
                "memory_id": "mem-5",
                "source_title": "Any",
                "source_quote": "",
                "text": "",
                "speaker": "user",
                "timestamp": "2026-06-26T12:00:00Z",
                "conversation_id": "conv-1",
                "project": "X",
            }
        )
        result = memory_cleanup.clean_point(point, seen_ids=set(), seen_hashes={}, include_vectors=False, repair_quotes=False)
        self.assertFalse(result.payload["safe_for_reflection"])
        self.assertIn("missing_quote", result.payload["reflection_blockers"])

    def test_vector_arrays_hidden_from_review_payload(self):
        point = self._point(
            payload={
                "memory_id": "mem-6",
                "source_title": "Any",
                "source_quote": "Valid quote with context for vector case.",
                "text": "Valid quote with context for vector case.",
                "speaker": "user",
                "timestamp": "2026-06-26T12:00:00Z",
                "conversation_id": "conv-1",
                "project": "X",
                "vector": [1.0, 2.0, 3.0],
            }
        )
        result = memory_cleanup.clean_point(point, seen_ids=set(), seen_hashes={}, include_vectors=False, repair_quotes=False)
        self.assertNotIn("vector", result.payload["payload_cleaned"])
        self.assertTrue(result.payload["vector_removed_from_payload"])

    def test_vector_metadata_recorded(self):
        result = memory_cleanup.clean_point(
            self._point(vector={"insight": [0.1, 0.2, 0.3]}),
            seen_ids=set(),
            seen_hashes={},
            include_vectors=False,
            repair_quotes=False,
        )
        self.assertTrue(result.payload["vector_present"])
        self.assertEqual(result.payload["vector_dimensions"], 3)

    def test_phase_applicability_rules(self):
        tech = self._point(
            payload={
                "memory_id": "mem-7",
                "source_title": "Python server setup",
                "source_quote": "Set up database and server code for deployment.",
                "text": "Set up database and server code for deployment.",
                "speaker": "user",
                "timestamp": "2026-06-26T12:00:00Z",
                "conversation_id": "conv-1",
                "project": "X",
            }
        )
        spiritual = self._point(
            payload={
                "memory_id": "mem-8",
                "source_title": "Operation New Earth journal",
                "source_quote": "Consciousness shifts and awakening insights are central here.",
                "text": "Consciousness shifts and awakening insights are central here.",
                "speaker": "user",
                "timestamp": "2026-06-26T12:00:00Z",
                "conversation_id": "conv-1",
                "project": "X",
            }
        )
        tech_result = memory_cleanup.clean_point(tech, seen_ids=set(), seen_hashes={}, include_vectors=False, repair_quotes=False)
        spiritual_result = memory_cleanup.clean_point(spiritual, seen_ids=set(), seen_hashes={}, include_vectors=False, repair_quotes=False)
        self.assertEqual(tech_result.payload["phase_applicability"], "not_applicable")
        self.assertIn(spiritual_result.payload["phase_applicability"], {"applicable", "uncertain"})

    def test_duplicate_record_detected(self):
        seen_ids = set()
        seen_hashes = {}
        first = memory_cleanup.clean_point(self._point(), seen_ids=seen_ids, seen_hashes=seen_hashes, include_vectors=False, repair_quotes=False)
        second = memory_cleanup.clean_point(self._point(), seen_ids=seen_ids, seen_hashes=seen_hashes, include_vectors=False, repair_quotes=False)
        self.assertEqual(first.payload["cleanup_status"], "clean")
        self.assertEqual(second.payload["cleanup_status"], "skipped_duplicate")

    def test_malformed_payload_does_not_crash(self):
        point = {"id": "pt-bad", "payload": "not-json"}
        result = memory_cleanup.clean_point(point, seen_ids=set(), seen_hashes={}, include_vectors=False, repair_quotes=False)
        self.assertIn("corrupted_json", result.payload["reflection_blockers"])

    def test_dry_run_writes_nothing(self):
        fake_point = self._point()
        with patch("frontpocket.memory_cleanup.iter_raw_points", return_value=[fake_point]), patch(
            "frontpocket.memory_cleanup.write_cleaned_batch"
        ) as mocked_write:
            code = memory_cleanup.run(["--dry-run"])
            self.assertEqual(code, 0)
            mocked_write.assert_not_called()

    def test_write_mode_writes_cleaned_records(self):
        fake_point = self._point()
        with patch("frontpocket.memory_cleanup.iter_raw_points", return_value=[fake_point]), patch(
            "frontpocket.memory_cleanup.write_cleaned_batch"
        ) as mocked_write:
            code = memory_cleanup.run(["--write-cleaned"])
            self.assertEqual(code, 0)
            self.assertTrue(mocked_write.called)


if __name__ == "__main__":
    unittest.main()
