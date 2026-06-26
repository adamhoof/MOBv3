#!/bin/bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <mdb_file_path> <output_csv_path>" >&2
  exit 1
fi

required_cols=("Nazev" "EAN" "ProdejDPH" "MJ2" "MJ2Koef" "StavZ")
output_file="$2"

tmp_file=$(mktemp)
trap 'rm -f "$tmp_file"' EXIT

mdb-export --delimiter=";" --quote='"' --escape-invisible "$1" SKz > "$tmp_file"

if [ ! -s "$tmp_file" ]; then
    echo "Error: Failed to export data from MDB file." >&2
    exit 1
fi

header_line=$(head -n 1 "$tmp_file")

declare -A required_map
for col_name in "${required_cols[@]}"; do
    norm_name=$(echo "$col_name" | tr '[:upper:]' '[:lower:]')
    required_map["$norm_name"]="$col_name"
done

declare -A found_indices
IFS=';' read -r -a header_fields <<< "$header_line"

for i in "${!header_fields[@]}"; do
    field_name_norm=$(echo "${header_fields[$i]}" | tr '[:upper:]' '[:lower:]' | tr -d '"' | tr -d " ")
    if [[ -v required_map[$field_name_norm] ]]; then
        original_name=${required_map[$field_name_norm]}
        found_indices["$original_name"]=$((i + 1))
    fi
done

if [ "${#found_indices[@]}" -ne "${#required_cols[@]}" ]; then
    echo "Error: Could not find all required columns in the MDB file header." >&2
    for req_col in "${required_cols[@]}"; do
        if [ -z "${found_indices[$req_col]:-}" ]; then
            echo "Missing column: '$req_col'" >&2
        fi
    done
    exit 1
fi

declare -a ordered_indices
for col_name in "${required_cols[@]}"; do
    ordered_indices+=(${found_indices[$col_name]})
done

output_header=$(IFS=';'; echo "${required_cols[*]}")
echo "$output_header" > "$output_file"

awk -F';' \
  -v c1="${ordered_indices[0]}" \
  -v c2="${ordered_indices[1]}" \
  -v c3="${ordered_indices[2]}" \
  -v c4="${ordered_indices[3]}" \
  -v c5="${ordered_indices[4]}" \
  -v c6="${ordered_indices[5]}" \
  'BEGIN { OFS=";" } NR>1 { print $c1, $c2, "\""$c3"\"", $c4, "\""$c5"\"", "\""$c6"\"" }' "$tmp_file" >> "$output_file"
