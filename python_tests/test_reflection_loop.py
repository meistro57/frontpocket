from __future__ import annotations

import unittest
from argparse import Namespace
from unittest.mock import patch

from frontpocket import reflection_loop


class ReflectionLoopTests(unittest.TestCase):
    def test_parser_defaults_to_from_cleaned(self):
        parser = reflection_loop.build_parser()
        args = parser.parse_args([])
        self.assertTrue(args.from_cleaned)

    def test_raw_input_flag_disables_from_cleaned(self):
        parser = reflection_loop.build_parser()
        args = parser.parse_args(["--raw-input"])
        self.assertFalse(args.from_cleaned)

    def test_goal_flag_supported_for_legacy_cli_compatibility(self):
        parser = reflection_loop.build_parser()
        args = parser.parse_args(["--goal", "map arc"])
        self.assertEqual(args.goal, "map arc")

    def test_run_once_skips_unsafe_cleaned_by_default(self):
        unsafe = reflection_loop.MemoryPoint(
            point_id="1",
            memory_id="m1",
            source_title="t",
            source_quote="valid quote but blocked",
            text="valid quote but blocked",
            speaker="user",
            source_role="user",
            timestamp="2026-06-26T00:00:00Z",
            conversation_id="c",
            project="p",
            safe_for_reflection=False,
        )
        args = Namespace(
            from_cleaned=True,
            include_needs_review=False,
            speaker=None,
            limit=10,
            quiet=True,
            from_scratch=False,
        )
        with patch("frontpocket.reflection_loop.ensure_target_collection"), patch(
            "frontpocket.reflection_loop.get_already_reflected", return_value=set()
        ), patch("frontpocket.reflection_loop.iter_cleaned_memory_points", return_value=[unsafe]), patch(
            "frontpocket.reflection_loop.reflect_on_point"
        ) as reflect_mock:
            state = reflection_loop.run_once(args, "model")
            self.assertEqual(state["processed"], 0)
            self.assertEqual(state["skipped"], 1)
            reflect_mock.assert_not_called()

    def test_run_once_uses_cleaned_iterator_by_default(self):
        safe = reflection_loop.MemoryPoint(
            point_id="1",
            memory_id="m1",
            source_title="t",
            source_quote="This is a sufficiently long safe quote for reflection.",
            text="This is a sufficiently long safe quote for reflection.",
            speaker="user",
            source_role="user",
            timestamp="2026-06-26T00:00:00Z",
            conversation_id="c",
            project="p",
            safe_for_reflection=True,
        )
        reflection = reflection_loop.Reflection(depth="moderate", reflection_confidence=0.7)
        args = Namespace(
            from_cleaned=True,
            include_needs_review=False,
            speaker=None,
            limit=10,
            quiet=True,
            from_scratch=False,
        )
        with patch("frontpocket.reflection_loop.ensure_target_collection"), patch(
            "frontpocket.reflection_loop.get_already_reflected", return_value=set()
        ), patch("frontpocket.reflection_loop.iter_cleaned_memory_points", return_value=[safe]) as cleaned_iter, patch(
            "frontpocket.reflection_loop.iter_raw_memory_points", return_value=[]
        ) as raw_iter, patch("frontpocket.reflection_loop.reflect_on_point", return_value=reflection), patch(
            "frontpocket.reflection_loop.upsert_reflection", return_value="id"
        ):
            state = reflection_loop.run_once(args, "model")
            self.assertEqual(state["processed"], 1)
            self.assertTrue(cleaned_iter.called)
            self.assertFalse(raw_iter.called)

    def test_confidence_caps_applied_by_quote_quality(self):
        self.assertEqual(reflection_loop.apply_confidence_cap("complete", 0.91), 0.91)
        self.assertEqual(reflection_loop.apply_confidence_cap("partial", 0.91), 0.75)
        self.assertEqual(reflection_loop.apply_confidence_cap("truncated", 0.88), 0.75)
        self.assertEqual(reflection_loop.apply_confidence_cap("malformed", 0.90), 0.35)


if __name__ == "__main__":
    unittest.main()
