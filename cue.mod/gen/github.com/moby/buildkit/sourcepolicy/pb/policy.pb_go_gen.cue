// Code generated by cue get go. DO NOT EDIT.

//cue:generate cue get go github.com/moby/buildkit/sourcepolicy/pb

package moby_buildkit_v1_sourcepolicy

// PolicyAction defines the action to take when a source is matched
#PolicyAction: _ // #enumPolicyAction

#enumPolicyAction:
	#PolicyAction_ALLOW |
	#PolicyAction_DENY |
	#PolicyAction_CONVERT

#values_PolicyAction: {
	PolicyAction_ALLOW:   #PolicyAction_ALLOW
	PolicyAction_DENY:    #PolicyAction_DENY
	PolicyAction_CONVERT: #PolicyAction_CONVERT
}

#PolicyAction_ALLOW:   #PolicyAction & 0
#PolicyAction_DENY:    #PolicyAction & 1
#PolicyAction_CONVERT: #PolicyAction & 2

// AttrMatch defines the condition to match a source attribute
#AttrMatch: _ // #enumAttrMatch

#enumAttrMatch:
	#AttrMatch_EQUAL |
	#AttrMatch_NOTEQUAL |
	#AttrMatch_MATCHES

#values_AttrMatch: {
	AttrMatch_EQUAL:    #AttrMatch_EQUAL
	AttrMatch_NOTEQUAL: #AttrMatch_NOTEQUAL
	AttrMatch_MATCHES:  #AttrMatch_MATCHES
}

#AttrMatch_EQUAL:    #AttrMatch & 0
#AttrMatch_NOTEQUAL: #AttrMatch & 1
#AttrMatch_MATCHES:  #AttrMatch & 2

// Match type is used to determine how a rule source is matched
#MatchType: _ // #enumMatchType

#enumMatchType:
	#MatchType_WILDCARD |
	#MatchType_EXACT |
	#MatchType_REGEX

#values_MatchType: {
	MatchType_WILDCARD: #MatchType_WILDCARD
	MatchType_EXACT:    #MatchType_EXACT
	MatchType_REGEX:    #MatchType_REGEX
}

// WILDCARD is the default matching type.
// It may first attempt to due an exact match but will follow up with a wildcard match
// For something more powerful, use REGEX
#MatchType_WILDCARD: #MatchType & 0

// EXACT treats the source identifier as a litteral string match
#MatchType_EXACT: #MatchType & 1

// REGEX treats the source identifier as a regular expression
// With regex matching you can also use match groups to replace values in the destination identifier
#MatchType_REGEX: #MatchType & 2

// Rule defines the action(s) to take when a source is matched
#Rule: {
	action?:   #PolicyAction    @go(Action) @protobuf(1,varint,opt,proto3,enum=moby.buildkit.v1.sourcepolicy.PolicyAction)
	selector?: null | #Selector @go(Selector,*Selector) @protobuf(2,bytes,opt,proto3)
	updates?:  null | #Update   @go(Updates,*Update) @protobuf(3,bytes,opt,proto3)
}

// Update contains updates to the matched build step after rule is applied
#Update: {
	identifier?: string @go(Identifier) @protobuf(1,bytes,opt,proto3)
	attrs?: {[string]: string} @go(Attrs,map[string]string) @protobuf(2,map[bytes]bytes,rep,proto3)
}

// Selector identifies a source to match a policy to
#Selector: {
	identifier?: string @go(Identifier) @protobuf(1,bytes,opt,proto3)

	// MatchType is the type of match to perform on the source identifier
	match_type?: #MatchType @go(MatchType) @protobuf(2,varint,opt,json=matchType,proto3,enum=moby.buildkit.v1.sourcepolicy.MatchType)
	constraints?: [...null | #AttrConstraint] @go(Constraints,[]*AttrConstraint) @protobuf(3,bytes,rep,proto3)
}

// AttrConstraint defines a constraint on a source attribute
#AttrConstraint: {
	key?:       string     @go(Key) @protobuf(1,bytes,opt,proto3)
	value?:     string     @go(Value) @protobuf(2,bytes,opt,proto3)
	condition?: #AttrMatch @go(Condition) @protobuf(3,varint,opt,proto3,enum=moby.buildkit.v1.sourcepolicy.AttrMatch)
}

// Policy is the list of rules the policy engine will perform
#Policy: {
	version?: int64 @go(Version) @protobuf(1,varint,opt,proto3)
	rules?: [...null | #Rule] @go(Rules,[]*Rule) @protobuf(2,bytes,rep,proto3)
}