# Example Purview Intake — a synthetic fixture, not anyone's purview

> This file exists so `reins-purview-complete-map` can be verified in a public repo with no
> estate content whatsoever. The vocabulary below is invented. A real purview — with a real
> operator's programs in it — belongs in a private repository and is supplied to this tool by path.

## The system that keeps this complete (anti-re-narrowing)

**1. The mandatory 4-axis frame.** ALTITUDE · PROJECTION · PROGRAM · LIFECYCLE, plus DECISIONS
and OBLIGATIONS. Dropping an axis IS re-narrowing.

**2. The high-recall mapping engine (re-runnable).**

**3. Coverage is machine-checkable.** Every mapped element has an intake item. An element with no
item is an ORPHAN equals a leak.

**4. The invariants that END re-narrowing.**

**5. Request-path operation.** New asks land as new items here first.

---

## INTAKE ITEMS

### example-program-one  (2 items)

- **ex-alpha** An example element
  - [~] axis:altitude-telos | status:partial | cap:operator | src:critic:altitude
  - Synthetic. Carries no meaning outside this fixture.
- **ex-beta** A second example element
  - [ ] axis:obligation | status:absent | cap:operator | src:critic:program
  - Synthetic.

### example-program-two  (1 items)

- **ex-gamma** A third example element
  - [x] axis:program | status:done | cap:operator | src:critic:meta-completeness
  - Synthetic.

## COVERAGE MATRIX

**Program coverage**

| key | items |
|---|---|
| example-program-one | 2 |
| example-program-two | 1 |

**Axis coverage**

| key | items |
|---|---|
| altitude-telos | 1 |
| obligation | 1 |
| program | 1 |

**Source / critic provenance**

| key | items |
|---|---|
| critic:altitude | 1 |
| critic:program | 1 |
| critic:meta-completeness | 1 |

**Orphan-check:** 3/3
