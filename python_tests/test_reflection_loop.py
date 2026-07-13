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

    def test_parser_accepts_workers_mode_flags(self):
        parser = reflection_loop.build_parser()
        args = parser.parse_args(["--mode", "workers", "--workers", "3", "--worker-id", "2"])
        self.assertEqual(args.mode, "workers")
        self.assertEqual(args.workers, 3)
        self.assertEqual(args.worker_id, 2)

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

    def test_worker_mode_processes_only_matching_shard(self):
        points = [
            reflection_loop.MemoryPoint(
                point_id="1",
                memory_id="m1",
                source_title="t1",
                source_quote="This is a sufficiently long safe quote for reflection one.",
                text="This is a sufficiently long safe quote for reflection one.",
                speaker="user",
                source_role="user",
                timestamp="2026-06-26T00:00:00Z",
                conversation_id="c",
                project="p",
                safe_for_reflection=True,
            ),
            reflection_loop.MemoryPoint(
                point_id="2",
                memory_id="m2",
                source_title="t2",
                source_quote="This is a sufficiently long safe quote for reflection two.",
                text="This is a sufficiently long safe quote for reflection two.",
                speaker="user",
                source_role="user",
                timestamp="2026-06-26T00:00:00Z",
                conversation_id="c",
                project="p",
                safe_for_reflection=True,
            ),
            reflection_loop.MemoryPoint(
                point_id="3",
                memory_id="m3",
                source_title="t3",
                source_quote="This is a sufficiently long safe quote for reflection three.",
                text="This is a sufficiently long safe quote for reflection three.",
                speaker="user",
                source_role="user",
                timestamp="2026-06-26T00:00:00Z",
                conversation_id="c",
                project="p",
                safe_for_reflection=True,
            ),
        ]
        reflection = reflection_loop.Reflection(depth="moderate", reflection_confidence=0.7)
        args = Namespace(
            from_cleaned=True,
            include_needs_review=False,
            speaker=None,
            limit=10,
            quiet=True,
            from_scratch=False,
        )
        expected = [p.memory_id for p in points if reflection_loop.worker_slot(p, 2) == 0]
        with patch("frontpocket.reflection_loop.ensure_target_collection"), patch(
            "frontpocket.reflection_loop.get_already_reflected", return_value=set()
        ), patch("frontpocket.reflection_loop.iter_cleaned_memory_points", return_value=points), patch(
            "frontpocket.reflection_loop.reflect_on_point", return_value=reflection
        ) as reflect_mock, patch("frontpocket.reflection_loop.upsert_reflection", return_value="id"):
            state = reflection_loop.run_once(args, "model", worker_index=0, worker_total=2)
            processed = [call.args[0].memory_id for call in reflect_mock.call_args_list]
            self.assertEqual(processed, expected)
            self.assertEqual(state["processed"], len(expected))

    def test_confidence_caps_applied_by_quote_quality(self):
        self.assertEqual(reflection_loop.apply_confidence_cap("complete", 0.91), 0.91)
        self.assertEqual(reflection_loop.apply_confidence_cap("partial", 0.91), 0.75)
        self.assertEqual(reflection_loop.apply_confidence_cap("truncated", 0.88), 0.75)
        self.assertEqual(reflection_loop.apply_confidence_cap("malformed", 0.90), 0.35)


if __name__ == "__main__":
    unittest.main()
