#!/usr/bin/env bash

set -eu

failures=0
successes=0

cd "$(dirname "$0")"

VERBOSE=""
while [[ $# -gt 0 ]]; do
	case $1 in
	-v|--verbose)
		VERBOSE="1"
		shift
		;;
	*)
		echo "Unexpected argument '$1'"
		exit 1
		;;
	esac
done

exec_case() {
	local test_case="$1"
	local test_case_err="$1.err"
	local test_case_out="$1.out"
	local ret
	local output
	local err

	ret=0
	kubectl apply --dry-run=server -f "$test_case" > "$test_case_out" 2>&1 || ret=$?
	output=$(cat "$test_case_out")

	if [ $ret == 1 ] && [ ! -f "${test_case_err}" ]; then
		echo "${test_case}: FAIL $output"
		((++failures))
	elif [ $ret == 1 ] && [ -f "$test_case_err" ]; then
		err=$(cat "$test_case_err")
		if grep "$err" "$test_case_out" > /dev/null 2>&1; then
			((++successes))
			if [ ! -z "$VERBOSE" ]; then echo "$test_case: SUCCESS (expected failure)"; fi
		else
			echo "$test_case: FAIL (unexpected msg): $output"
			((++failures))
		fi
	elif [ $ret == 0 ] && [ -f "$test_case_err" ]; then
		echo "$test_case: FAIL (unexpected success): $output"
		((++failures))
	elif [ $ret == 0 ] && [ ! -f "$test_case_err" ]; then
		if [ ! -z "$VERBOSE" ]; then echo "$test_case: SUCCESS"; fi
		((++successes))
	fi
}

exec_tx_case() {
	local test_case_pre="$1"
	local test_case_post="${1/pre/post}"
	local test_case_out="${1/pre.yaml/post.yaml.out}"
	local test_case_err="${1/pre.yaml/post.yaml.tx_err}"
	local ret=0
	local output

	if ! kubectl apply -f "$test_case_pre" > /dev/null 2>&1; then
		echo "$test_case_pre: FAIL applying"
		((++failures))
		return
	fi
	kubectl apply -f "$test_case_post" > "$test_case_out" 2>&1 || ret=$?
	kubectl delete -f "$test_case_post" > /dev/null 2>&1
	output=$(cat "$test_case_out")

	local test_header="${test_case_pre} -> ${test_case_post}:"

	if [ $ret == 1 ] && [ ! -f "${test_case_err}" ]; then
		echo "$test_header FAIL $output"
		((++failures))
	elif [ $ret == 1 ] && [ -f "$test_case_err" ]; then
		local err
		err=$(cat "$test_case_err")
		if grep "$err" "$test_case_out" > /dev/null 2>&1; then
			if [ ! -z "$VERBOSE" ]; then echo "$test_header SUCCESS (expected failure)"; fi
			((++successes))
		else
			echo "$test_header FAIL (unexpected msg): $output"
			((++failures))
		fi
	elif [ $ret == 0 ] && [ -f "$test_case_err" ]; then
		echo "$test_header FAIL (unexpected success): $output"
		((++failures))
	elif [ $ret == 0 ] && [ ! -f "$test_case_err" ]; then
		if [ ! -z "$VERBOSE" ]; then echo "$test_header SUCCESS"; fi
		((++successes))
	fi
}

while IFS= read -r test_case; do
	exec_case "$test_case"
done < <(find cel-tests -name \*.yaml)

while IFS= read -r test_case; do
	exec_tx_case "$test_case"
done < <(find cel-tests -name \*.pre.yaml)

echo
echo "SUCCESS: ${successes}"
echo "FAILURES: ${failures}"

if [ "${failures}" != 0 ]; then
	exit 1
fi
