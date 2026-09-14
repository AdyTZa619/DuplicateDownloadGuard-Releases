from __future__ import annotations

import hashlib

import pytest

from cinecalendar.updater import (
    POST_UPDATE_MODE,
    health_matches,
    is_newer_version,
    parse_manifest,
    parse_special_startup,
    sha256_path,
    write_health_marker,
)


def valid_manifest(**overrides):
    data = {
        "version": "2.0.1",
        "url": "https://example.invalid/CineCalendar_LATEST.exe",
        "sha256": "a" * 64,
        "notes": "test",
        "publishedAt": "2026-09-14T00:00:00Z",
        "channel": "stable",
    }
    data.update(overrides)
    return data


def test_version_compare_is_numeric_not_lexicographic():
    assert is_newer_version("2.0.1", "2.0.0")
    assert is_newer_version("2.10.0", "2.9.9")
    assert is_newer_version("3.0", "2.99.99")
    assert not is_newer_version("2.0.0", "2.0.0")
    assert not is_newer_version("1.9.9", "2.0.0")


def test_manifest_requires_https_hash_and_stable_channel():
    info = parse_manifest(valid_manifest())
    assert info.version == "2.0.1"
    assert info.channel == "stable"
    assert info.sha256 == "a" * 64

    with pytest.raises(ValueError):
        parse_manifest(valid_manifest(url="http://example.invalid/file.exe"))
    with pytest.raises(ValueError):
        parse_manifest(valid_manifest(sha256="1234"))
    with pytest.raises(ValueError):
        parse_manifest(valid_manifest(channel="test"))


def test_sha256_path_matches_known_digest(tmp_path):
    p = tmp_path / "payload.exe"
    payload = b"CineCalendar updater payload\x00\x01"
    p.write_bytes(payload)
    assert sha256_path(p) == hashlib.sha256(payload).hexdigest()


def test_health_marker_is_version_bound(tmp_path):
    p = tmp_path / "health.ok"
    write_health_marker(p, "2.0.1")
    assert health_matches(p, "2.0.1")
    assert not health_matches(p, "2.0.0")


def test_post_update_mode_does_not_enter_updater_helper(tmp_path):
    health = tmp_path / "health.ok"
    exit_code, post = parse_special_startup([
        "CineCalendar.exe", POST_UPDATE_MODE, str(health), "2.0.1"
    ])
    assert exit_code is None
    assert post == (str(health), "2.0.1")
