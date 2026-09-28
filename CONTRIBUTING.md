# Contributing to htmlbag

Thank you for taking the time to send a patch. htmlbag is developed in a
private mono repository together with boxes and glue, glu and XTS, and
mirrored to this one. That is why pull requests are squash merged and
the commit that lands here does not carry your commit history. Your name
stays in the squashed commit.

## Before you start

Whether a change works is only half the question. The other half is
whether htmlbag should carry it for years: glu, XTS and bagme render
their documents with it, and every property it reads is one that
documents come to rely on. So some changes are welcome as a pull request
right away, and some need an agreement first.

**Send a pull request directly for:**

- **A bug fix.** htmlbag crashes, renders HTML or CSS differently from
  the specification in a way that is not a documented deviation, or
  gives a different result from run to run or from platform to
  platform.
- **CSS as the specification describes it.** A property, value or
  selector that htmlbag does not support yet, implemented as the CSS
  specification defines it.
- **Documentation and tests.**

**Open an issue first, and wait for an answer before you write the code,
for:**

- a property or at-rule of htmlbag's own (the `-bag-` prefix),
- a deviation from the CSS specification, including a default other
  than the specified initial value,
- a change of the output of existing documents that is not a bug fix,
- new exported API (functions, types, fields), since other programs
  build on it,
- a change that needs pull requests in more than one repository.

Describe the problem in the issue, not only the solution: the HTML and
CSS you have, what you expect (a browser rendering helps), and what you
propose. It is much cheaper to agree on the shape of a feature before
anyone implements it. "No" and "not now" are possible answers, and they
say nothing about the quality of the idea.

**Keep pull requests small**, one change each. A pull request that
depends on an unreleased change in boxes and glue waits until that
change is released; name the dependency at the top of the description.

## What a change needs

1. **A test** that fails without the change: a small piece of HTML and
   CSS, and a check of the node list or the PDF it produces. Look at the
   existing `*_test.go` files for helpers.

2. **An entry in `Properties`** (`propertyspec.go`) for every property or
   value you add: the accepted values, an example, and in `Note` every
   limit or deviation from the CSS specification. The CSS reference of
   glu, XTS and the boxes and glue website is generated from this list,
   and `TestPropertySpecCoverage` fails when a property the code reads
   has no entry.

3. **A word on the output.** If documents that do not use your change
   render differently afterwards, say so in the pull request and why.
   The PDFs of glu, XTS and bagme are compared byte for byte before a
   release, so an unexplained change will be found, but it is easier to
   review when it is expected.

## Building and testing

```
go vet ./...
go test ./...
```

Format the code with `gofmt`.

## Releases

Merged changes are collected and released together, not one release per
pull request. A change that depends on a new version of boxes and glue
ships with the next htmlbag release after that version.

## Reporting bugs

Open an issue with a minimal piece of HTML and CSS that shows the
problem, the htmlbag version (or the version of the program you use it
with, such as glu or XTS), and what you expected to see. A screenshot of
the wrong output and of a browser rendering helps.
