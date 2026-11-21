#!/usr/bin/env python3
"""Exercise the synthetic PayGuard booking flow twice against a local stack."""

import argparse
import json
import os
from datetime import datetime, timedelta, timezone
from urllib.error import HTTPError
from urllib.request import Request, urlopen
from uuid import uuid4


class BookingAPI:
    def __init__(self, base, token):
        self.base = base.rstrip("/") + "/api/v1/booking"
        self.token = token

    def request(self, path, status, body=None, key=None):
        headers = {"Accept": "application/json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        if key:
            headers["Idempotency-Key"] = key
        payload = None
        if body is not None:
            headers["Content-Type"] = "application/json"
            payload = json.dumps(body).encode()
        request = Request(self.base + path, data=payload, headers=headers)
        try:
            with urlopen(request, timeout=20) as response:
                actual, data = response.status, json.load(response)
        except HTTPError as error:
            actual, data = error.code, json.load(error)
        assert actual == status, f"{request.method} {path}: expected {status}, received {actual}: {data}"
        return data

    def book(self, occurred_at, **fields):
        event_id = str(uuid4())
        body = dict(eventId=event_id, schemaVersion=1, stage="booking", occurredAt=occurred_at, **fields)
        return body, self.request("/events", 201, body, key=event_id)

    def assess(self, event, mode="mock", status=201):
        return self.request(f"/events/{event['eventId']}/assessment", status, {"jevMode": mode})

    def lineage(self, event, status=200):
        return self.request(f"/events/{event['eventId']}/lineage", status)

    def outcome(self, event, body, status=201):
        return self.request(f"/events/{event['eventId']}/outcomes", status, body)


def clock():
    # Vary the historical anchor while retaining identical relative fixtures so
    # repeat runs on the same merchant do not contribute to each other's velocity.
    anchor = datetime(2000, 1, 1, tzinfo=timezone.utc)
    anchor += timedelta(seconds=uuid4().int % 700_000_000)

    def at(offset):
        return (anchor + timedelta(seconds=offset)).isoformat().replace("+00:00", "Z")

    return at


def attributes(assessment):
    return {feature["name"]: feature for feature in assessment["features"]}


def assert_no_labels(data):
    if isinstance(data, dict):
        forbidden = {"groundtruth", "label", "isfraud", "hiddenringid", "outcomes", "eligible"}
        assert not forbidden.intersection(key.replace("_", "").lower() for key in data), data
        for value in data.values():
            assert_no_labels(value)
    elif isinstance(data, list):
        for value in data:
            assert_no_labels(value)


def run_fixture(api, other, profile_version):
    at = clock()
    normal, accepted = api.book(at(0), product="flight_ticket", amountMinor=0, sdkStatus="observed")
    assert api.request("/events", 200, normal, key=normal["eventId"]) == accepted
    api.request("/events", 409, {**normal, "product": "hotel_room"}, key=normal["eventId"])
    api.request("/events", 409, {**normal, "sdkStatus": "unavailable"}, key=normal["eventId"])
    api.request("/events", 400, {**normal, "sdkStatus": "unknown"}, key=normal["eventId"])
    api.request("/events", 400, {**normal, "label": "normal"}, key=normal["eventId"])
    other.lineage(normal, status=404)
    other.assess(normal, status=404)
    assert api.lineage(normal)["assessment"] is None
    normal_result = api.assess(normal)
    assert normal_result["outcome"] == "LOW_RISK", normal_result
    assert attributes(normal_result)["bookingVelocity60s"]["value"] == 1, normal_result
    assert normal_result["profileVersion"] == profile_version
    assert api.assess(normal, status=200) == normal_result
    amount = attributes(normal_result)["amountMinor"]
    assert amount["value"] == 0 and amount["presence"] == "OBSERVED", amount
    assert api.lineage(normal)["booking"]["sdkStatus"] == "observed"
    assert api.lineage(normal)["booking"]["product"] == "flight_ticket"

    for offset in (60, 61, 90, 91, 119, 121):
        api.book(at(offset))
    review, _ = api.book(at(120), product="physical")
    late, _ = api.book(at(118))
    result = api.assess(review)
    assert result["outcome"] == "REVIEW" and result["jev"]["answer"] == "ELEVATED", result
    assert result["ruleHits"] == ["BOOKING_VELOCITY_60S"], result
    assert attributes(result)["bookingVelocity60s"]["value"] == 5, result
    assert api.lineage(late)["booking"]["occurredAt"] == at(118)
    assert api.lineage(review)["booking"]["sdkStatus"] == "unknown"
    assert api.assess(review, status=200) == result

    timeout, _ = api.book(at(240), product="digital", sdkStatus="unavailable")
    timeout_result = api.assess(timeout, mode="timeout")
    assert timeout_result["outcome"] == "UNRESOLVED", timeout_result
    assert timeout_result["handoff"] == "MANUAL_REVIEW", timeout_result
    assert timeout_result["jev"]["status"] == "TIMEOUT", timeout_result
    assert timeout_result["jev"]["answer"] == "UNAVAILABLE", timeout_result
    assert attributes(timeout_result)["bookingVelocity60s"]["value"] == 1, timeout_result
    assert "paymentAction" not in timeout_result
    assert api.lineage(timeout)["booking"]["sdkStatus"] == "unavailable"
    missing = attributes(timeout_result)["amountMinor"]
    assert missing["value"] is None and missing["presence"] == "MISSING", missing
    assert api.assess(timeout, mode="timeout", status=200) == timeout_result
    api.assess(timeout, status=409)

    for assessment in (normal_result, result, timeout_result):
        assert assessment["jev"]["modelVersion"] and assessment["jev"]["rubricVersion"]
        assert assessment["ruleVersion"] and assessment["profileVersion"] == profile_version
        assert_no_labels(assessment)

    def signature(assessment):
        return (
            assessment["outcome"], assessment["handoff"], assessment["ruleHits"],
            assessment["jev"], attributes(assessment)["bookingVelocity60s"]["value"],
        )

    signatures = [signature(assessment) for assessment in (normal_result, result, timeout_result)]
    return signatures, normal, normal_result, timeout, at


def outcome_body(source, label, occurred_at):
    return dict(
        outcomeId=str(uuid4()), source=source, sourceReference=f"smoke-{uuid4()}",
        label=label, occurredAt=occurred_at,
    )


def parse_timestamp(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def run_baseline(api, other, threshold, normal, normal_result, timeout, at):
    analyst = outcome_body("analyst", "normal", at(30))
    chargeback = outcome_body("chargeback", "fraud", at(20))
    initial = api.outcome(normal, analyst)
    assert initial["outcome"]["eligible"] is False, initial
    initial_baseline = initial["baseline"]
    assert api.outcome(normal, analyst, status=200) == initial
    api.outcome(normal, {**analyst, "label": "fraud"}, status=409)
    api.outcome(normal, {**analyst, "outcomeId": str(uuid4())}, status=409)
    other.outcome(normal, outcome_body("verified_demo", "normal", at(40)), status=404)
    observed = api.outcome(normal, chargeback)
    assert observed["outcome"]["eligible"] is False
    assert observed["baseline"] == initial_baseline
    failed = api.outcome(timeout, outcome_body("verified_demo", "normal", at(250)))
    assert failed["outcome"]["eligible"] is False
    assert failed["baseline"] == initial_baseline
    history = api.lineage(normal)
    assert history["assessment"] == normal_result
    assert [row["source"] for row in history["outcomes"]] == ["chargeback", "analyst"], history
    earlier, later = history["outcomes"]
    assert parse_timestamp(earlier["occurredAt"]) < parse_timestamp(later["occurredAt"])
    assert parse_timestamp(earlier["receivedAt"]) >= parse_timestamp(later["receivedAt"])

    pending, _ = api.book(at(360))
    premature = outcome_body("verified_demo", "normal", at(361))
    api.outcome(pending, premature, status=409)
    api.outcome(pending, {**premature, "eligible": True}, status=400)
    assert api.lineage(pending)["outcomes"] == []
    baseline = initial_baseline
    for index in range(threshold):
        offset = 480 + index * 120
        sample, _ = api.book(at(offset))
        api.assess(sample)
        body = outcome_body("verified_demo", "normal", at(offset + 1))
        imported = api.outcome(sample, body)
        assert imported["outcome"]["eligible"] is True, imported
        assert imported["baseline"]["sampleCount"] == baseline["sampleCount"] + 1, imported
        assert api.outcome(sample, body, status=200) == imported
        duplicate = api.outcome(sample, outcome_body("verified_demo", "normal", at(offset + 2)))
        assert duplicate["baseline"] == imported["baseline"], duplicate
        api.outcome(sample, outcome_body("verified_demo", "fraud", at(offset + 3)), status=409)
        baseline = imported["baseline"]
        if baseline["sampleCount"] < threshold:
            assert baseline["state"] == "COLD_START" and baseline["version"] is None
        else:
            assert baseline["state"] == "ACTIVE" and baseline["version"], baseline

    assert baseline["sampleCount"] == initial_baseline["sampleCount"] + threshold
    assert baseline["version"] != initial_baseline["version"]
    delayed = api.assess(pending)["baseline"]
    assert delayed["version"] == initial_baseline["version"] and delayed["state"] == initial_baseline["state"]
    next_event, _ = api.book(at(480 + threshold * 120))
    next_result = api.assess(next_event)
    assert next_result["baseline"] == baseline, next_result
    assert next_result["outcome"] == "LOW_RISK", next_result
    assert next_result["ruleVersion"] == normal_result["ruleVersion"]
    assert_no_labels(next_result)
    assert api.assess(normal, status=200) == normal_result
    assert api.outcome(normal, analyst, status=200) == initial
    print(f"Baseline: {initial_baseline['state']} → ACTIVE, +{threshold} distinct verified-normal samples")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base", default="http://127.0.0.1:8081", help="Local PayGuard API URL")
    parser.add_argument("--merchant", choices=("a", "b"), default="a", help="Merchant to populate with synthetic fixtures")
    parser.add_argument("--threshold", type=int, default=3, help="Configured demo baseline minimum on the API")
    args = parser.parse_args()
    if args.threshold < 1:
        parser.error("--threshold must be positive")
    token_a = os.environ.get("PAYGUARD_DEMO_MERCHANT_A_TOKEN")
    token_b = os.environ.get("PAYGUARD_DEMO_MERCHANT_B_TOKEN")
    if not token_a or not token_b or token_a == token_b:
        parser.error("Set distinct PAYGUARD_DEMO_MERCHANT_A_TOKEN and PAYGUARD_DEMO_MERCHANT_B_TOKEN.")
    token, other_token = (token_a, token_b) if args.merchant == "a" else (token_b, token_a)
    api, other = BookingAPI(args.base, token), BookingAPI(args.base, other_token)
    setup = dict(schemaVersion=1, businessModel="ota", stage="booking", availableSignals=["occurredAt", "amountMinor"])
    path = "/profiles/proposal"
    BookingAPI(args.base, None).request(path, 401, setup)
    proposal = api.request(path, 200, setup)
    assert api.request(path, 200, setup) == proposal
    approved = api.request("/profiles/approve", 200, proposal)["profileVersion"]

    first, *_ = run_fixture(api, other, approved)
    second, normal, normal_result, timeout, at = run_fixture(api, other, approved)
    assert first == second, (first, second)
    print("Both fixture passes: LOW_RISK at velocity 1, REVIEW at velocity 5, UNRESOLVED at velocity 1")
    run_baseline(api, other, args.threshold, normal, normal_result, timeout, at)
    print("Booking smoke passed: replay, tenant scope, lineage, SDK missingness, and no QA labels in assessments")


if __name__ == "__main__":
    main()
