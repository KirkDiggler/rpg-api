package authoringv1alpha1_test

import (
	"google.golang.org/grpc/codes"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
)

// armingCatalog is what the rulebook says a placed monster may be armed with:
// simple then martial, each in the rulebook's own registry order. The tests
// below compare the wire to THIS and never to a list typed out here, so a
// rulebook that grows a weapon needs no edit in rpg-api -- which is the whole
// reason this RPC exists.
func armingCatalog() []weapons.Weapon {
	return append(weapons.GetSimpleWeapons(), weapons.GetMartialWeapons()...)
}

// TestListWeapons_Unauthenticated: authoring is a signed-in verb, ungated or
// not.
func (s *HandlerSuite) TestListWeapons_Unauthenticated() {
	_, err := s.handler.ListWeapons(s.T().Context(), &authoringpb.ListWeaponsRequest{})
	s.requireCode(err, codes.Unauthenticated)
}

// TestListWeapons_NeverTouchesTheRegistry is the ungating, made mechanical:
// the catalog is a property of the SERVER'S rulebook build, not of any stored
// content, so a build with no dungeons at all still has one. The registry
// mock expects nothing -- a call to it fails the test.
func (s *HandlerSuite) TestListWeapons_NeverTouchesTheRegistry() {
	resp, err := s.handler.ListWeapons(s.ctx, &authoringpb.ListWeaponsRequest{})
	s.Require().NoError(err)
	s.Require().NotEmpty(resp.GetWeapons())
}

// TestListWeapons_IsTheRulebooksOwnCatalogVerbatim runs over the rulebook's
// accessors rather than over the weapons somebody remembered, so a weapon
// added later cannot skip it.
//
// Every value on the wire is compared to the value the rulebook exports, ref
// included. There is no rpg-api copy of a weapon's name, its category word or
// its ref, and this is what says so: a handler that title-cased a name,
// sorted the list, spelled a ref by hand or derived `ranged` from the
// category word would fail here.
func (s *HandlerSuite) TestListWeapons_IsTheRulebooksOwnCatalogVerbatim() {
	resp, err := s.handler.ListWeapons(s.ctx, &authoringpb.ListWeaponsRequest{})
	s.Require().NoError(err)

	all := armingCatalog()
	s.Require().NotEmpty(all, "this build can arm a monster with nothing at all -- the catalog is broken, not the wire")
	s.Require().Len(resp.GetWeapons(), len(all))

	for i, want := range all {
		got := resp.GetWeapons()[i]

		wantRef := refs.Weapons.ByID(want.ID)
		s.Require().NotNil(wantRef, "the catalog offers %q but the refs namespace does not name it", want.ID)
		s.Require().Equal(wantRef.String(), got.GetRef())

		s.Require().Equal(want.Name, got.GetName())
		s.Require().Equal(string(want.Category), got.GetCategory())
		s.Require().Equal(want.IsRanged(), got.GetRanged(),
			"%q travels with the rulebook's own predicate, not a reading of its category word", want.ID)
	}
}

// TestListWeapons_EveryRefIsAWeaponRefTheAuthorCanWrite is the load-bearing
// half a field-for-field copy cannot give: the string on the wire is the
// string a placement's `actions:` list carries, so it has to PARSE as a
// weapon ref and the id inside it has to name a weapon this build has.
//
// A palette that emitted a blank, a bare id or "dnd5e:weapon:shortbow" would
// write a file the compiler refuses, and the author would learn it on save
// rather than here.
func (s *HandlerSuite) TestListWeapons_EveryRefIsAWeaponRefTheAuthorCanWrite() {
	resp, err := s.handler.ListWeapons(s.ctx, &authoringpb.ListWeaponsRequest{})
	s.Require().NoError(err)

	for _, got := range resp.GetWeapons() {
		ref, parseErr := core.ParseString(got.GetRef())
		s.Require().NoError(parseErr, "%q is not a ref at all", got.GetRef())
		s.Equal(refs.Module, ref.Module, "ref %q", got.GetRef())
		s.Equal(refs.TypeWeapons, ref.Type, "ref %q", got.GetRef())

		weapon, lookupErr := weapons.GetByID(ref.ID)
		s.Require().NoError(lookupErr, "ref %q names no weapon in this build", got.GetRef())
		s.Equal(weapon.Name, got.GetName(), "ref %q and name %q disagree", got.GetRef(), got.GetName())
	}
}

// TestListWeapons_ArmsTheMonstersTheDesignNames names the five weapons the
// monster-weapons design actually arms something with (rpg-project#448): the
// goblin's scimitar and shortbow, the thug's mace and heavy crossbow, the
// bandit's light crossbow. Named rather than left to the verbatim sweep above
// so a build that lost one, or that called a bow melee, fails on a line that
// says which.
//
// The ranged flags here are written out on purpose. The sweep above proves
// the wire equals the rulebook; this proves the rulebook is right about the
// five that matter, and the two together are what a single flipped bool
// cannot survive.
func (s *HandlerSuite) TestListWeapons_ArmsTheMonstersTheDesignNames() {
	resp, err := s.handler.ListWeapons(s.ctx, &authoringpb.ListWeaponsRequest{})
	s.Require().NoError(err)

	byRef := map[string]*authoringpb.WeaponDescriptor{}
	for _, w := range resp.GetWeapons() {
		byRef[w.GetRef()] = w
	}

	for _, tc := range []struct {
		ref      string
		name     string
		ranged   bool
		category string
	}{
		{"dnd5e:weapons:shortbow", "Shortbow", true, "simple-ranged"},
		{"dnd5e:weapons:light-crossbow", "Light Crossbow", true, "simple-ranged"},
		{"dnd5e:weapons:heavy-crossbow", "Heavy Crossbow", true, "martial-ranged"},
		{"dnd5e:weapons:scimitar", "Scimitar", false, "martial-melee"},
		{"dnd5e:weapons:mace", "Mace", false, "simple-melee"},
	} {
		got, offered := byRef[tc.ref]
		s.Require().True(offered, "the palette cannot arm a monster with %s", tc.ref)
		s.Equal(tc.name, got.GetName(), "%s", tc.ref)
		s.Equal(tc.ranged, got.GetRanged(), "%s attacks at range: %v", tc.ref, tc.ranged)
		s.Equal(tc.category, got.GetCategory(), "%s", tc.ref)
	}
}

// TestListWeapons_OffersNoSpecialWeapon pins the one thing this RPC decides:
// the unarmed strike is in the rulebook's catalog and is NOT an arming
// choice. The rulebook itself keeps it out of the category accessors as "not
// equippable" and this handler writes no filter of its own -- so this test
// also fails the day rpg-api starts deciding what a weapon is.
func (s *HandlerSuite) TestListWeapons_OffersNoSpecialWeapon() {
	_, inTheCatalog := weapons.All[weapons.UnarmedStrike]
	s.Require().True(inTheCatalog, "this test is about an EXCLUSION; the rulebook has to have the weapon")

	resp, err := s.handler.ListWeapons(s.ctx, &authoringpb.ListWeaponsRequest{})
	s.Require().NoError(err)

	unarmed := refs.Weapons.ByID(weapons.UnarmedStrike)
	s.Require().NotNil(unarmed)
	for _, got := range resp.GetWeapons() {
		s.NotEqual(unarmed.String(), got.GetRef(), "the palette offers a weapon nobody can be armed with")
	}
}

// TestListWeapons_IsGroupedAndStable pins the ordering the palette sections
// its chips by: every weapon of a category arrives together, in the order
// simple-melee, simple-ranged, martial-melee, martial-ranged, and the same
// request twice gives the same answer. A handler that ranged over the
// catalog's map -- the obvious way to write this -- would pass every test
// above and fail this one, most of the time.
func (s *HandlerSuite) TestListWeapons_IsGroupedAndStable() {
	first, err := s.handler.ListWeapons(s.ctx, &authoringpb.ListWeaponsRequest{})
	s.Require().NoError(err)
	second, err := s.handler.ListWeapons(s.ctx, &authoringpb.ListWeaponsRequest{})
	s.Require().NoError(err)

	order := func(resp *authoringpb.ListWeaponsResponse) []string {
		refStrings := make([]string, 0, len(resp.GetWeapons()))
		for _, w := range resp.GetWeapons() {
			refStrings = append(refStrings, w.GetRef())
		}

		return refStrings
	}
	s.Equal(order(first), order(second), "the same build answered the same question two different ways")

	var groups []string
	for _, w := range first.GetWeapons() {
		if len(groups) == 0 || groups[len(groups)-1] != w.GetCategory() {
			groups = append(groups, w.GetCategory())
		}
	}
	s.Equal([]string{
		string(weapons.CategorySimpleMelee),
		string(weapons.CategorySimpleRanged),
		string(weapons.CategoryMartialMelee),
		string(weapons.CategoryMartialRanged),
	}, groups, "a category arrives once, in one run")
}
