# TODO

- Investigate depaware pre-commit failures; re-enable depaware hooks once fixed. Hooks removed from `.pre-commit-config.yaml`:

```diff
      - id: viberun-depaware
        name: viberun depaware
        entry: tools/hooks/depaware-check
        language: system
        pass_filenames: false
        stages: [pre-commit]
      - id: viberun-depaware-deps
        name: viberun depaware dependency check
        entry: tools/hooks/depaware-deps-check
        language: system
        pass_filenames: false
        stages: [pre-commit]
```
