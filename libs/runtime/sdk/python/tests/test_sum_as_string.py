from celerity_runtime_sdk import sum_as_string


def test_sum_as_string():
    assert sum_as_string(1, 2) == "3"
