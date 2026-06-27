from observr._traceparent import parse_traceparent, format_traceparent


def test_parse_valid_header():
    result = parse_traceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
    assert result == ("4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7")


def test_parse_returns_none_for_bad_format():
    assert parse_traceparent("bad") is None
    assert parse_traceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7") is None  # 3 parts
    assert parse_traceparent("01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01") is None  # version


def test_parse_returns_none_for_short_fields():
    assert parse_traceparent("00-tooshort-00f067aa0ba902b7-01") is None
    assert parse_traceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-short-01") is None
    assert parse_traceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-1") is None


def test_parse_returns_none_for_uppercase_hex():
    assert parse_traceparent("00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01") is None
    assert parse_traceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00F067AA0BA902B7-01") is None


def test_parse_returns_none_for_all_zeros():
    assert parse_traceparent("00-00000000000000000000000000000000-00f067aa0ba902b7-01") is None
    assert parse_traceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01") is None


def test_format_produces_correct_header():
    result = format_traceparent("4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7")
    assert result == "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
