/// The cats wire protocol in pure Dart.
///
/// # Import this with a prefix
///
/// ```dart
/// import 'package:catsproto/catsproto.dart' as cats;
/// ```
///
/// `Key`, `Theme`, `Title` and `Image` are wire message types here AND Flutter
/// widgets there. The collision is intentional: these are the SERVER's names,
/// and renaming them would put the wire and the code reading it out of step for
/// the convenience of one import line.
///
/// # What is generated
///
/// Everything under `src/generated/` comes from `cmd/catgen-dart` in the cats
/// repo, from the Go definitions that are the actual source of truth. Do not
/// edit it; regenerate:
///
/// ```
/// cd ../cats && go run ./cmd/catgen-dart \
///     -out ../cats-mobile/packages/catsproto/lib/src/generated
/// ```
///
/// The revision it was generated from is pinned in this repo's `CATS_REV`.
library;

export 'src/connection.dart';
export 'src/endpoint.dart';
export 'src/generated/attrs.g.dart';
export 'src/generated/codec.g.dart';
export 'src/generated/commands.g.dart';
export 'src/generated/keys.g.dart';
export 'src/generated/wire.g.dart';
export 'src/grid.dart';
export 'src/session.dart';
export 'src/sha256.dart';
